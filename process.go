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

	imports := importNames(f)

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

		scope := &testScope{
			tIdents: testingTIdents(testVar, funcDecl.Body),
			locals:  localNames(funcDecl),
			imports: imports,
		}

		insertTParallel(funcDecl.Body, testVar, scope, needFixLoopVar, true)

		return true
	})

	// gofmt
	var fmtedBuf bytes.Buffer
	if err := format.Node(&fmtedBuf, fs, f); err != nil {
		return nil, fmt.Errorf("gofmt error occurred. %w", err)
	}

	return fmtedBuf.Bytes(), nil
}

// insertTParallel inserts Parallel() into the test whose body is body and whose
// *testing.T is named testVar, then does the same for each of its subtests,
// wherever they are started: at the top level, in a loop, in an if block, or
// in another subtest. Parallel() is inserted into body itself only if
// mayParallel. It reports whether Parallel() was inserted into body.
func insertTParallel(body *ast.BlockStmt, testVar string, scope *testScope, needFixLoopVar, mayParallel bool) bool {
	// Insert Parallel() unless the test already calls it, or the test (or
	// any of its subtests, at any depth) calls t.Setenv/t.Chdir etc. or
	// changes global state: see deniesParallel.
	insert := mayParallel && !callsParallel(body, testVar) && !deniesParallel(body, scope)

	// Parallel subtests run after body returns, so they would see global
	// state that body changed already restored, e.g. by a deferred call.
	subtestsMayParallel := !changesGlobalStateOutsideSubtests(body, testVar, scope)

	ast.PreorderStack(body, nil, func(n ast.Node, stack []ast.Node) bool {
		// A t.Run inside a closure such as a helper can run several times,
		// with captured state changed in between, so it is left alone.
		// Subtest callbacks are handled at their t.Run call below.
		if _, ok := n.(*ast.FuncLit); ok {
			return false
		}

		if !hasRunMethod(n, testVar) {
			return true
		}

		// n is a call to t.Run. Its subtest is handled by the recursive
		// call below, with the subtest's own *testing.T name.
		fun := getRunCallback(n)
		innerTestVar := getRunCallbackParameterName(n)

		// No usable *testing.T name, e.g. `func(_ *testing.T)`.
		if fun == nil || innerTestVar == "" {
			return false
		}

		// A parallel subtest's t.Run returns before the subtest finishes, so
		// one whose result is used (`if !t.Run(...)`) must stay sequential.
		// So must one that captures a variable the test reassigns: every
		// parallel subtest would see the last value.
		_, resultDiscarded := stack[len(stack)-1].(*ast.ExprStmt)
		mayParallel := subtestsMayParallel && resultDiscarded && !assignsCapturedVar(body, fun, nil)

		if insertTParallel(fun.Body, innerTestVar, scope, needFixLoopVar, mayParallel) && needFixLoopVar {
			copyLoopVars(stack, fun)
		}

		return false
	})

	if insert {
		tpStmt := buildTParallelStmt(body.Lbrace, testVar)
		body.List = append([]ast.Stmt{tpStmt}, body.List...)
	}

	return insert
}

// callsParallel reports whether the test whose body is body calls
// <testVar>.Parallel() anywhere outside its subtests, including inside blocks
// such as `if !testing.Short() { t.Parallel() }`. Calling Parallel() twice
// panics.
func callsParallel(body *ast.BlockStmt, testVar string) bool {
	var found bool

	ast.Inspect(body, func(n ast.Node) bool {
		// Do not descend into subtests; their Parallel() calls belong to
		// the subtest, not to this test.
		if hasRunMethod(n, testVar) {
			return false
		}

		if hasParallelMethod(n, testVar) {
			found = true
		}

		return !found
	})

	return found
}

// assignsCapturedVar reports whether node assigns a variable that fun
// references, other than those in except. Examples are want in `want = 2`
// between two subtests, i in `for i < 3 { ...; i++ }` and tc in
// `for _, tc = range xs`: a parallel subtest runs after its parent returns and
// would see the last value. The post statement of a loop that declares the
// variable (the i++ in `for i := 0; ...; i++`) does not count: from Go 1.22
// every iteration gets its own i, and copyLoopVars copies it before.
// Assignments inside fun itself do not count.
func assignsCapturedVar(node ast.Node, fun *ast.FuncLit, except []string) bool {
	var captured bool

	check := func(exprs ...ast.Expr) {
		for _, expr := range exprs {
			id, ok := expr.(*ast.Ident)
			if ok && !slices.Contains(except, id.Name) && identReferenced(fun.Body, id.Name) {
				captured = true
			}
		}
	}

	ast.Inspect(node, func(m ast.Node) bool {
		if captured || m == fun {
			return false
		}

		switch s := m.(type) {
		case *ast.AssignStmt:
			if s.Tok != token.DEFINE {
				check(s.Lhs...)
			}
		case *ast.IncDecStmt:
			check(s.X)
		case *ast.RangeStmt:
			if s.Tok == token.ASSIGN {
				check(s.Key, s.Value)
			}
		case *ast.ForStmt:
			if s.Post != nil {
				captured = assignsCapturedVar(s.Post, fun, append(slices.Clone(except), forLoopVarNames(s)...))
			}

			for _, part := range []ast.Node{s.Init, s.Body} {
				if !captured && part != nil {
					captured = assignsCapturedVar(part, fun, except)
				}
			}

			return false
		}

		return !captured
	})

	return captured
}

// copyLoopVars copies, at the top of each enclosing loop in stack, the loop
// variables that the now parallel subtest fun captures: `tc := tc`. Before Go
// 1.22 every iteration shares the same loop variable
// (https://go.dev/doc/go1.22#language).
func copyLoopVars(stack []ast.Node, fun *ast.FuncLit) {
	for _, n := range stack {
		var (
			loopVars []string
			body     *ast.BlockStmt
		)

		switch loop := n.(type) {
		case *ast.RangeStmt:
			loopVars, body = rangeLoopVarNames(loop), loop.Body
		case *ast.ForStmt:
			loopVars, body = forLoopVarNames(loop), loop.Body
		default:
			continue
		}

		copied := loopVarsCopiedInBody(body, loopVars)

		var copies []ast.Stmt

		for _, name := range loopVars {
			if identReferenced(fun.Body, name) && !copied[name] {
				copies = append(copies, buildLoopVarReAssignmentStmt(body.Lbrace, name))
			}
		}

		body.List = append(copies, body.List...)
	}
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

// globalStateFuncs are package-level functions, by import path, that change
// process-wide state. A test calling one races with every other parallel test.
var globalStateFuncs = map[string]map[string]bool{
	"os":       {"Setenv": true, "Unsetenv": true, "Clearenv": true, "Chdir": true},
	"log":      {"SetOutput": true, "SetFlags": true, "SetPrefix": true},
	"log/slog": {"SetDefault": true, "SetLogLoggerLevel": true},
}

// testScope holds what deniesParallel needs to know about the names used in a
// test function.
type testScope struct {
	// tIdents are the names of every *testing.T value in scope: the test
	// function's own parameter plus the parameter of every subtest closure.
	tIdents map[string]bool
	// locals are the names declared anywhere inside the test function.
	// Assigning any other name changes a package-level variable.
	locals map[string]bool
	// imports maps the name a package is referred to by in the file to its
	// import path.
	imports map[string]string
}

// importNames maps the name each package imported by f is referred to by to
// its import path. The name of an import without an alias is guessed from the
// last path element, skipping a major version suffix such as v2.
func importNames(f *ast.File) map[string]string {
	names := map[string]string{}

	for _, spec := range f.Imports {
		path := strings.Trim(spec.Path.Value, `"`)

		if spec.Name != nil {
			names[spec.Name.Name] = path

			continue
		}

		elems := strings.Split(path, "/")
		name := elems[len(elems)-1]

		if len(elems) > 1 && isMajorVersionSuffix(name) {
			name = elems[len(elems)-2]
		}

		names[name] = path
	}

	return names
}

// isMajorVersionSuffix reports whether s is a module major version path
// element such as v2.
func isMajorVersionSuffix(s string) bool {
	if len(s) < 2 || s[0] != 'v' {
		return false
	}

	for _, r := range s[1:] {
		if !unicode.IsDigit(r) {
			return false
		}
	}

	return true
}

// localNames returns the names declared anywhere inside funcDecl: parameters
// and results of the function and of every function literal in it, and every
// variable declared with :=, var or range. Scopes are not resolved: a local
// variable shadowing a package-level one hides assignments to the latter.
func localNames(funcDecl *ast.FuncDecl) map[string]bool {
	names := map[string]bool{}

	addFields := func(fields *ast.FieldList) {
		if fields == nil {
			return
		}

		for _, field := range fields.List {
			for _, name := range field.Names {
				names[name.Name] = true
			}
		}
	}

	addIdents := func(exprs ...ast.Expr) {
		for _, expr := range exprs {
			if id, ok := expr.(*ast.Ident); ok {
				names[id.Name] = true
			}
		}
	}

	addFields(funcDecl.Type.Params)
	addFields(funcDecl.Type.Results)

	ast.Inspect(funcDecl.Body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.FuncLit:
			addFields(n.Type.Params)
			addFields(n.Type.Results)
		case *ast.AssignStmt:
			if n.Tok == token.DEFINE {
				addIdents(n.Lhs...)
			}
		case *ast.RangeStmt:
			if n.Tok == token.DEFINE {
				addIdents(n.Key, n.Value)
			}
		case *ast.ValueSpec:
			for _, name := range n.Names {
				names[name.Name] = true
			}
		}

		return true
	})

	return names
}

// changesGlobalState reports whether n changes process-wide state: it calls
// one of globalStateFuncs, or assigns to (or increments) a package-level
// variable, of this package or another one, or a field or element of one.
func (s *testScope) changesGlobalState(n ast.Node) bool {
	var lhs []ast.Expr

	switch n := n.(type) {
	case *ast.CallExpr:
		sel, ok := n.Fun.(*ast.SelectorExpr)
		if !ok {
			return false
		}

		pkg, ok := sel.X.(*ast.Ident)

		return ok && !s.locals[pkg.Name] && globalStateFuncs[s.imports[pkg.Name]][sel.Sel.Name]
	case *ast.AssignStmt:
		if n.Tok == token.DEFINE {
			return false
		}

		lhs = n.Lhs
	case *ast.IncDecStmt:
		lhs = []ast.Expr{n.X}
	default:
		return false
	}

	for _, expr := range lhs {
		if root := rootIdent(expr); root != nil && root.Name != "_" && !s.locals[root.Name] {
			return true
		}
	}

	return false
}

// changesGlobalStateOutsideSubtests reports whether the test whose body is
// body, and whose *testing.T is named testVar, changes global state itself,
// outside the subtests it starts.
func changesGlobalStateOutsideSubtests(body *ast.BlockStmt, testVar string, scope *testScope) bool {
	var changes bool

	ast.Inspect(body, func(n ast.Node) bool {
		if changes || hasRunMethod(n, testVar) {
			return false
		}

		changes = scope.changesGlobalState(n)

		return !changes
	})

	return changes
}

// rootIdent returns the variable that expr, the left-hand side of an
// assignment, is part of: v in v, v.f, v[i] and v.f[i].g. It returns nil when
// expr is not rooted in a variable, as in *p or f().x.
func rootIdent(expr ast.Expr) *ast.Ident {
	for {
		switch e := expr.(type) {
		case *ast.Ident:
			return e
		case *ast.SelectorExpr:
			expr = e.X
		case *ast.IndexExpr:
			expr = e.X
		case *ast.ParenExpr:
			expr = e.X
		default:
			return nil
		}
	}
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
// cryptotest.SetGlobalRandom(t, ...) on any *testing.T named in scope.tIdents,
// or code that changes global state (see changesGlobalState), which races
// with other parallel tests. Nested subtests are searched on purpose: a
// Setenv in a subtest also forbids Parallel() in every enclosing test.
func deniesParallel(node ast.Node, scope *testScope) bool {
	var denies bool

	ast.Inspect(node, func(n ast.Node) bool {
		if denies {
			return false
		}

		if scope.changesGlobalState(n) {
			denies = true

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
			if recv, ok := sel.X.(*ast.Ident); ok && scope.tIdents[recv.Name] {
				denies = true
			}
		}

		if parallelDenyingFuncs[sel.Sel.Name] {
			for _, arg := range call.Args {
				if id, ok := arg.(*ast.Ident); ok && scope.tIdents[id.Name] {
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
// statement (key and/or value), skipping the blank identifier. A range
// statement that assigns existing variables (`for _, tc = range`) declares
// none.
func rangeLoopVarNames(r *ast.RangeStmt) []string {
	if r.Tok != token.DEFINE {
		return nil
	}

	var names []string

	for _, expr := range []ast.Expr{r.Key, r.Value} {
		if id, ok := expr.(*ast.Ident); ok && id.Name != "_" {
			names = append(names, id.Name)
		}
	}

	return names
}

// forLoopVarNames returns the names of the variables declared by the init
// statement of a three-clause for loop, such as i in `for i := 0; ...`.
func forLoopVarNames(f *ast.ForStmt) []string {
	assign, ok := f.Init.(*ast.AssignStmt)
	if !ok || assign.Tok != token.DEFINE {
		return nil
	}

	var names []string

	for _, expr := range assign.Lhs {
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

// loopVarsCopiedInBody returns the loop variables that the loop body already
// re-declares with the `tc := tc` idiom (or `i, tc := i, tc`) directly at the
// top level of the body.
func loopVarsCopiedInBody(body *ast.BlockStmt, loopVars []string) map[string]bool {
	copied := map[string]bool{}

	for _, stmt := range body.List {
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
