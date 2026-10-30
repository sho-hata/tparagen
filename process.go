package tparagen

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/build/constraint"
	"go/format"
	"go/parser"
	"go/token"
	"go/version"
	"slices"
	"strings"
	"unicode"
)

const (
	testMethodPackageType = "testing"
	testMethodStruct      = "T"
	testPrefix            = "Test"
)

// GenerateTParallel processes a Go source file to ensure that test functions
// and subtests call t.Parallel() where appropriate. It parses the provided
// source code, inspects the AST for test functions, and inserts t.Parallel()
// calls if they are missing. Additionally, it handles cases where test functions
// use t.Setenv() and ensures proper handling of loop variables in subtests.
//
// Returns:
// - A byte slice containing the modified source code.
// - An error if any issues occur during parsing or formatting.
func GenerateTParallel(filename string, src []byte, needFixLoopVar bool) ([]byte, error) {
	fs := token.NewFileSet()

	f, err := parser.ParseFile(fs, filename, src, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("cannot parse file. %w", err)
	}

	if !isTparagenTargetFile(f) {
		return src, nil
	}

	// A `//go:build go1.N` line sets the language version of this file, so
	// it can need loop variable copies even when go.mod does not.
	if !needFixLoopVar {
		needFixLoopVar = buildConstraintNeedsLoopVarFix(f)
	}

	ast.Inspect(f, func(n ast.Node) bool {
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok {
			return true
		}

		// Check nolint target
		if !isTparagenTargetFunc(funcDecl.Doc) {
			return true
		}

		// Check runs for test functions only
		isTest, testVar := isTestFunction(funcDecl)
		if !isTest {
			return true
		}

		// Per-function state. It must start fresh for every test function:
		// otherwise a Parallel() call found in one function would suppress
		// the insertion in every function that follows it in the file.
		var testHasParallel bool

		// Names of every *testing.T value in scope within this function: the
		// function's own parameter plus the parameter of every subtest closure.
		tIdents := testingTIdents(testVar, funcDecl.Body)

		// Look through the whole body, not only its top-level statements: a
		// Parallel() call inside a block such as `if !testing.Short() { ... }`
		// counts too, and calling Parallel() twice panics.
		ast.Inspect(funcDecl.Body, func(n ast.Node) bool {
			// Do not descend into subtests; their Parallel() calls
			// belong to the subtest, not to this function.
			if hasRunMethod(n, testVar) {
				return false
			}

			if hasParallelMethod(n, testVar) {
				testHasParallel = true
			}

			return !testHasParallel
		})

		var rangeStmts []*ast.RangeStmt

		for _, l := range funcDecl.Body.List {
			switch s := l.(type) {
			case *ast.ExprStmt:
				ast.Inspect(s, func(n ast.Node) bool {
					// Check if the t.Run within the test function is calling t.Parallel
					if !hasRunMethod(n, testVar) {
						return true
					}

					// n is a call to t.Run; find out the name of the subtest's *testing.T parameter.
					innerTestVar := getRunCallbackParameterName(n)
					if innerTestVar == "" {
						return true
					}

					fun := getRunCallback(n)
					if fun == nil {
						return true
					}

					var subTestHasParallel bool

					ast.Inspect(s, func(p ast.Node) bool {
						if !subTestHasParallel {
							subTestHasParallel = hasParallelMethod(p, innerTestVar)
						}

						return true
					})

					// Insert Parallel() unless the subtest already calls it, or the
					// subtest (or a nested subtest) calls t.Setenv/t.Chdir etc.,
					// which panic when the test or any ancestor is parallel.
					if !subTestHasParallel && !deniesParallel(fun.Body, tIdents) {
						tpStmt := buildTParallelStmt(fun.Body.Lbrace, innerTestVar)
						fun.Body.List = append([]ast.Stmt{tpStmt}, fun.Body.List...)
					}

					return true
				})

			case *ast.RangeStmt:
				rangeStmts = append(rangeStmts, s)
			}
		}

		// Insert Parallel() unless the test already calls it, or the test (or
		// any of its subtests, at any depth) calls t.Setenv/t.Chdir etc.:
		// those panic when the test or any of its ancestors is parallel.
		if !testHasParallel && !deniesParallel(funcDecl.Body, tIdents) {
			tpStmt := buildTParallelStmt(funcDecl.Body.Lbrace, testVar)
			funcDecl.Body.List = append([]ast.Stmt{tpStmt}, funcDecl.Body.List...)
		}

		// Every range loop is handled on its own: a test may run several
		// tables of subtests one after another.
		for _, r := range rangeStmts {
			insertTParallelInRangeSubtests(r, testVar, tIdents, needFixLoopVar)
		}

		return true
	})

	// gofmt
	var fmtedBuf bytes.Buffer
	if err := format.Node(&fmtedBuf, fs, f); err != nil {
		return nil, fmt.Errorf("gofmt error occurred. %w", err)
	}

	return fmtedBuf.Bytes(), nil
}

// insertTParallelInRangeSubtests inserts Parallel() into the subtests started
// directly in the body of the range loop r, unless one of the loop's subtests
// already calls Parallel() or forbids it. Before Go 1.22 (needFixLoopVar) the
// loop variables captured by those subtests are copied as well.
func insertTParallelInRangeSubtests(r *ast.RangeStmt, testVar string, tIdents map[string]bool, needFixLoopVar bool) {
	var (
		runExists, hasParallel, denies bool

		// Loop variables declared by r, and the subset that is referenced
		// from a subtest callback inside it.
		loopVars          = rangeLoopVarNames(r)
		loopVarsUsedInRun = map[string]bool{}
	)

	ast.Inspect(r, func(n ast.Node) bool {
		stmt, ok := n.(*ast.ExprStmt)
		if !ok || !hasRunMethod(stmt.X, testVar) {
			return true
		}
		// stmt.X is a call to Run(); find out the name of the subtest's *testing.T parameter.
		innerTestVar := getRunCallbackParameterName(stmt.X)

		runExists = true

		if !hasParallel {
			hasParallel = methodParallelIsCalledInMethodRun(stmt.X, innerTestVar)
		}

		if fun := getRunCallback(stmt.X); fun != nil {
			if !denies {
				denies = deniesParallel(fun.Body, tIdents)
			}

			for _, name := range loopVars {
				if identReferenced(fun.Body, name) {
					loopVarsUsedInRun[name] = true
				}
			}
		}

		return true
	})

	if !runExists || hasParallel || denies {
		return
	}

	for _, stmt := range r.Body.List {
		e, ok := stmt.(*ast.ExprStmt)
		if !ok || !hasRunMethod(e.X, testVar) {
			continue
		}

		fun := getRunCallback(e.X)
		innerTestVar := getRunCallbackParameterName(e.X)
		// No usable *testing.T name, e.g. `func(_ *testing.T)`.
		if fun == nil || innerTestVar == "" {
			continue
		}

		tpStmt := buildTParallelStmt(fun.Body.Lbrace, innerTestVar)
		fun.Body.List = append([]ast.Stmt{tpStmt}, fun.Body.List...)
	}

	// Before Go 1.22 every iteration shares the same loop variable
	// (https://go.dev/doc/go1.22#language), so a parallel subtest
	// that captures it needs its own copy: `tc := tc`.
	if !needFixLoopVar {
		return
	}

	copied := loopVarsCopiedInBody(r, loopVars)

	var copies []ast.Stmt

	for _, name := range loopVars {
		if loopVarsUsedInRun[name] && !copied[name] {
			copies = append(copies, buildLoopVarReAssignmentStmt(r.Body.Lbrace, name))
		}
	}

	r.Body.List = append(copies, r.Body.List...)
}

// Checks if the function has the param type *testing.T; if it does, then the
// parameter name is returned, too.
func isTestFunction(funcDecl *ast.FuncDecl) (bool, string) {
	if !strings.HasPrefix(funcDecl.Name.Name, testPrefix) {
		return false, ""
	}

	// A declaration without a body (e.g. implemented elsewhere) has nothing
	// to insert into.
	if funcDecl.Body == nil {
		return false, ""
	}

	if funcDecl.Type.Params == nil || len(funcDecl.Type.Params.List) != 1 {
		return false, ""
	}

	param := funcDecl.Type.Params.List[0]

	// `func TestX(*testing.T)` has no name to call Parallel() on, and neither
	// has `func TestX(_ *testing.T)`: `_.Parallel()` does not compile.
	if len(param.Names) != 1 || param.Names[0].Name == "_" {
		return false, ""
	}

	starExp, ok := param.Type.(*ast.StarExpr)
	if !ok {
		return false, ""
	}

	selectExpr, ok := starExp.X.(*ast.SelectorExpr)
	if !ok {
		return false, ""
	}

	if selectExpr.Sel.Name != testMethodStruct {
		return false, ""
	}

	s, ok := selectExpr.X.(*ast.Ident)
	if !ok {
		return false, ""
	}

	return s.Name == testMethodPackageType, param.Names[0].Name
}

func exprCallHasMethod(node ast.Node, receiverName, methodName string) bool {
	if n, ok := node.(*ast.CallExpr); ok {
		if fun, ok := n.Fun.(*ast.SelectorExpr); ok {
			if receiver, ok := fun.X.(*ast.Ident); ok {
				return receiver.Name == receiverName && fun.Sel.Name == methodName
			}
		}
	}

	return false
}

func hasParallelMethod(node ast.Node, testVar string) bool {
	return exprCallHasMethod(node, testVar, "Parallel")
}

func hasRunMethod(node ast.Node, testVar string) bool {
	return exprCallHasMethod(node, testVar, "Run")
}

// parallelDenyingMethods are the *testing.T methods that panic when the test,
// or any of its ancestors, has called Parallel() (see checkParallel in the
// testing package).
var parallelDenyingMethods = map[string]bool{
	"Setenv": true, // Go 1.17+
	"Chdir":  true, // Go 1.24+
}

// parallelDenyingFuncs are functions taking a *testing.T argument that have
// the same constraint as parallelDenyingMethods.
var parallelDenyingFuncs = map[string]bool{
	"SetGlobalRandom": true, // testing/cryptotest, Go 1.26+
}

// testingTIdents returns the names of every *testing.T value in scope within
// body: root (the test function's own parameter) plus the *testing.T parameter
// of every function literal declared inside body, i.e. subtest callbacks.
func testingTIdents(root string, body ast.Node) map[string]bool {
	idents := map[string]bool{root: true}

	ast.Inspect(body, func(n ast.Node) bool {
		fun, ok := n.(*ast.FuncLit)
		if !ok {
			return true
		}

		for _, field := range fun.Type.Params.List {
			if !isTestingT(field.Type) {
				continue
			}

			for _, name := range field.Names {
				idents[name.Name] = true
			}
		}

		return true
	})

	return idents
}

// isTestingT reports whether expr is the type expression *testing.T.
func isTestingT(expr ast.Expr) bool {
	star, ok := expr.(*ast.StarExpr)
	if !ok {
		return false
	}

	sel, ok := star.X.(*ast.SelectorExpr)
	if !ok {
		return false
	}

	pkg, ok := sel.X.(*ast.Ident)

	return ok && pkg.Name == testMethodPackageType && sel.Sel.Name == testMethodStruct
}

// deniesParallel reports whether node contains a call that forbids Parallel()
// on the enclosing test and on all of its ancestors: t.Setenv, t.Chdir, or
// cryptotest.SetGlobalRandom(t, ...) on any *testing.T named in tIdents.
// Nested subtests are searched on purpose: a Setenv in a subtest also forbids
// Parallel() in every enclosing test.
func deniesParallel(node ast.Node, tIdents map[string]bool) bool {
	var denies bool

	ast.Inspect(node, func(n ast.Node) bool {
		if denies {
			return false
		}

		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		if parallelDenyingMethods[sel.Sel.Name] {
			if recv, ok := sel.X.(*ast.Ident); ok && tIdents[recv.Name] {
				denies = true
			}
		}

		if parallelDenyingFuncs[sel.Sel.Name] {
			for _, arg := range call.Args {
				if id, ok := arg.(*ast.Ident); ok && tIdents[id.Name] {
					denies = true
				}
			}
		}

		return !denies
	})

	return denies
}

// In an expression of the form t.Run(x, func(q *testing.T) {...}), return the
// function literal that is the subtest body, or nil if there is none.
func getRunCallback(node ast.Node) *ast.FuncLit {
	call, ok := node.(*ast.CallExpr)
	if !ok || len(call.Args) < 2 {
		return nil
	}

	fun, _ := call.Args[1].(*ast.FuncLit)

	return fun
}

// In an expression of the form t.Run(x, func(q *testing.T) {...}), return the
// value "q". In _most_ code, the name is probably t, but we shouldn't just
// assume.
func getRunCallbackParameterName(node ast.Node) string {
	if n, ok := node.(*ast.CallExpr); ok {
		if len(n.Args) < 2 {
			// We want argument #2, but this call doesn't have two
			// arguments. Maybe it's not really t.Run.
			return ""
		}

		funcArg := n.Args[1]

		if fun, ok := funcArg.(*ast.FuncLit); ok {
			if len(fun.Type.Params.List) < 1 {
				// Subtest function doesn't have any parameters.
				return ""
			}

			firstArg := fun.Type.Params.List[0]
			// We'll assume firstArg.Type is *testing.T.
			if len(firstArg.Names) < 1 {
				return ""
			}

			// `func(_ *testing.T)`: there is no name to call Parallel() on.
			if firstArg.Names[0].Name == "_" {
				return ""
			}

			return firstArg.Names[0].Name
		}
	}

	return ""
}

func methodParallelIsCalledInMethodRun(node ast.Node, testVar string) bool {
	var isCalledParallel bool

	if callExp, ok := node.(*ast.CallExpr); ok {
		for _, arg := range callExp.Args {
			if !isCalledParallel {
				ast.Inspect(arg, func(n ast.Node) bool {
					if !isCalledParallel {
						isCalledParallel = hasParallelMethod(n, testVar)

						return true
					}

					return false
				})
			}
		}
	}

	return isCalledParallel
}

// build `<testVar>.Parallel()` statement to pos location specified in the argument.
func buildTParallelStmt(pos token.Pos, testVar string) *ast.ExprStmt {
	return &ast.ExprStmt{
		X: &ast.CallExpr{
			Fun: &ast.SelectorExpr{
				X: &ast.Ident{
					NamePos: pos,
					Name:    testVar,
				},
				Sel: &ast.Ident{
					NamePos: pos,
					Name:    "Parallel",
				},
			},
			Lparen: pos,
			Rparen: pos,
		},
	}
}

func buildLoopVarReAssignmentStmt(pos token.Pos, varName string) *ast.AssignStmt {
	return &ast.AssignStmt{
		Lhs: []ast.Expr{
			&ast.Ident{
				NamePos: pos,
				Name:    varName,
			},
		},
		TokPos: pos,
		Tok:    token.DEFINE,
		Rhs: []ast.Expr{
			&ast.Ident{
				NamePos: pos,
				Name:    varName,
			},
		},
	}
}

// rangeLoopVarNames returns the names of the variables declared by a range
// statement (key and/or value), skipping the blank identifier.
func rangeLoopVarNames(r *ast.RangeStmt) []string {
	var names []string

	for _, expr := range []ast.Expr{r.Key, r.Value} {
		if id, ok := expr.(*ast.Ident); ok && id.Name != "_" {
			names = append(names, id.Name)
		}
	}

	return names
}

// identReferenced reports whether an identifier called name occurs anywhere in
// node. Shadowing is not resolved on purpose: a false positive only leads to a
// harmless extra `name := name` copy.
func identReferenced(node ast.Node, name string) bool {
	var found bool

	ast.Inspect(node, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && id.Name == name {
			found = true
		}

		return !found
	})

	return found
}

// loopVarsCopiedInBody returns the loop variables that the range body already
// re-declares with the `tc := tc` idiom (or `i, tc := i, tc`) directly at the
// top level of the body.
func loopVarsCopiedInBody(r *ast.RangeStmt, loopVars []string) map[string]bool {
	copied := map[string]bool{}

	for _, stmt := range r.Body.List {
		assign, ok := stmt.(*ast.AssignStmt)
		if !ok || assign.Tok != token.DEFINE || len(assign.Lhs) != len(assign.Rhs) {
			continue
		}

		for i := range assign.Lhs {
			lhs, ok := assign.Lhs[i].(*ast.Ident)
			if !ok {
				continue
			}

			rhs, ok := assign.Rhs[i].(*ast.Ident)
			if !ok || lhs.Name != rhs.Name {
				continue
			}

			if slices.Contains(loopVars, lhs.Name) {
				copied[lhs.Name] = true
			}
		}
	}

	return copied
}

// nolintTargets are the linter names that, when listed in a nolint directive,
// exclude the file or function from tparagen.
var nolintTargets = map[string]bool{
	"all":          true,
	"paralleltest": true,
	"tparallel":    true,
}

// hasNolintCommentDirective reports whether the comment group contains a nolint
// directive that applies to tparagen. Any other comment, including an ordinary
// doc comment, does not count.
func hasNolintCommentDirective(cg *ast.CommentGroup) bool {
	for _, c := range cg.List {
		if isNolintDirectiveForTparagen(c.Text) {
			return true
		}
	}

	return false
}

// isNolintDirectiveForTparagen reports whether a single comment line is a
// nolint directive that applies to tparagen: a bare `//nolint`, `//nolint:all`,
// or one naming paralleltest/tparallel such as `//nolint:tparallel,paralleltest`.
// The legacy space-separated form `//nolint tparallel` and a trailing
// explanation (`//nolint:tparallel // reason`) are accepted too.
func isNolintDirectiveForTparagen(text string) bool {
	const prefix = "//nolint"

	if !strings.HasPrefix(text, prefix) {
		return false
	}

	rest := strings.TrimPrefix(text, prefix)

	// Drop a trailing explanation.
	if i := strings.Index(rest, "//"); i >= 0 {
		rest = rest[:i]
	}

	rest = strings.TrimPrefix(strings.TrimSpace(rest), ":")

	linters := strings.FieldsFunc(rest, func(r rune) bool {
		return r == ',' || unicode.IsSpace(r)
	})

	// A bare //nolint disables every linter.
	if len(linters) == 0 {
		return true
	}

	for _, l := range linters {
		if nolintTargets[l] {
			return true
		}
	}

	return false
}

// buildConstraintNeedsLoopVarFix reports whether the file's //go:build line
// pins it to a Go version before loopVarFixVersion, where every iteration
// shares the loop variable.
func buildConstraintNeedsLoopVarFix(f *ast.File) bool {
	for _, cg := range f.Comments {
		if cg.Pos() > f.Package {
			break
		}

		for _, c := range cg.List {
			if !constraint.IsGoBuild(c.Text) {
				continue
			}

			expr, err := constraint.Parse(c.Text)
			if err != nil {
				return false
			}

			v := constraint.GoVersion(expr)

			return v != "" && version.Compare(v, loopVarFixVersion) < 0
		}
	}

	return false
}

// isTparagenTargetFile reports whether the file should be processed at all.
// A nolint directive placed anywhere before the package clause (on its own,
// after a build constraint or a license header, or as the package doc comment)
// excludes the whole file.
func isTparagenTargetFile(f *ast.File) bool {
	for _, cg := range f.Comments {
		if cg.Pos() > f.Package {
			break
		}

		if hasNolintCommentDirective(cg) {
			return false
		}
	}

	return true
}

// if a function has a nolint comment, the function is removed from the target.
// also, if a specific linter (tparallel, paralleltest) is specified with a nolint comment, it is removed from the target.
func isTparagenTargetFunc(funcComment *ast.CommentGroup) bool {
	if funcComment == nil {
		return true
	}

	return !hasNolintCommentDirective(funcComment)
}
