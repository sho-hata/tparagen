package tparagen

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"go/types"
	"strings"
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

	if !isTparagenTargetFile(f.Comments) {
		return src, nil
	}

	typesInfo := &types.Info{Defs: map[*ast.Ident]types.Object{}}

	var (
		testHasSetenv   bool
		testHasParallel bool
	)

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

		for _, l := range funcDecl.Body.List {
			if s, ok := l.(*ast.ExprStmt); ok {
				ast.Inspect(s, func(n ast.Node) bool {
					// Check if the Run() within the test function is calling t.Parallel
					if hasRunMethod(n, testVar) {
						return false
					}

					// Check if the test method is calling Parallel()
					// If Parallel() is inserted once in a subtest in subsequent processing, `funcHasParallelmethod`  is true.
					if !testHasParallel {
						testHasParallel = hasParallelMethod(n, testVar)
					}

					// Check if the test method is calling Setenv()
					// If Setenv() is inserted once in a subtest in subsequent processing, `funcHasParallelmethod`  is true.
					if !testHasSetenv {
						testHasSetenv = hasSetenvMethod(n, testVar)
					}

					return true
				})
			}
		}

		var (
			rangeStatementOverTestCasesExists,
			rangeStatementHasParallelMethod,
			rangeStatementHasSetEnvMethod,
			loopVarReInitialized bool

			loopVariableUsedInRun *string

			rangeNode ast.Node
		)

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

					var subTestHasParallel, subTestHasSetEnv bool

					ast.Inspect(s, func(p ast.Node) bool {
						if !subTestHasParallel {
							subTestHasParallel = hasParallelMethod(p, innerTestVar)
						}
						if !subTestHasSetEnv {
							subTestHasSetEnv = hasSetenvMethod(p, innerTestVar)
						}

						return true
					})

					// Check if the sub test calls t.Parallel.
					if !subTestHasParallel && !subTestHasSetEnv {
						if n, ok := n.(*ast.CallExpr); ok {
							funcArg := n.Args[1]
							// insert parallel helper method
							if fun, ok := funcArg.(*ast.FuncLit); ok {
								tpStmt := buildTParallelStmt(fun.Body.Lbrace)
								fun.Body.List = append([]ast.Stmt{tpStmt}, fun.Body.List...)
							}
						}
					}

					return true
				})

			// Check if the range over testcases is calling t.Parallel
			case *ast.RangeStmt:
				rangeNode = s

				var loopVars []types.Object
				for _, expr := range []ast.Expr{s.Key, s.Value} {
					if id, ok := expr.(*ast.Ident); ok {
						loopVars = append(loopVars, typesInfo.ObjectOf(id))
					}
				}

				ast.Inspect(s, func(n ast.Node) bool {
					switch n := n.(type) {
					case *ast.ExprStmt:
						if !hasRunMethod(n.X, testVar) {
							return true
						}
						// e.X is a call to Run(); find out the name of the subtest's *testing.T parameter.
						innerTestVar := getRunCallbackParameterName(n.X)

						rangeStatementOverTestCasesExists = true

						if !rangeStatementHasParallelMethod {
							rangeStatementHasParallelMethod = methodParallelIsCalledInMethodRun(n.X, innerTestVar)
						}

						if !rangeStatementHasSetEnvMethod {
							rangeStatementHasSetEnvMethod = methodSetEnvIsCalledInMethodRun(n.X, innerTestVar)
						}

						if loopVariableUsedInRun == nil {
							if run, ok := n.X.(*ast.CallExpr); ok {
								loopVariableUsedInRun = loopVarReferencedInRun(run, loopVars, typesInfo)
							}
						}
					case *ast.AssignStmt:
						if !loopVarReInitialized {
							loopVarReInitialized = loopVarReAssigned(n, loopVars, typesInfo)
						}
					}

					return true
				})
			}
		}

		// Check if the main test calls Parallel().
		if !testHasParallel && !testHasSetenv {
			tpStmt := buildTParallelStmt(funcDecl.Body.Lbrace)
			funcDecl.Body.List = append([]ast.Stmt{tpStmt}, funcDecl.Body.List...)
		}

		// Check if the sub tests calls t.Parallel.
		if rangeNode != nil && rangeStatementOverTestCasesExists && !rangeStatementHasParallelMethod && !rangeStatementHasSetEnvMethod {
			var isInsertedTparallel bool

			ast.Inspect(rangeNode, func(n ast.Node) bool {
				if isInsertedTparallel {
					return true
				}

				if r, ok := rangeNode.(*ast.RangeStmt); ok {
					for _, n := range r.Body.List {
						if e, ok := n.(*ast.ExprStmt); ok {
							if !hasRunMethod(e.X, testVar) {
								continue
							}

							if c, ok := e.X.(*ast.CallExpr); ok {
								if len(c.Args) != 2 {
									return true
								}
								funcArg := c.Args[1]
								// insert parallel helper method
								if fun, ok := funcArg.(*ast.FuncLit); ok {
									tpStmt := buildTParallelStmt(fun.Body.Lbrace)
									fun.Body.List = append([]ast.Stmt{tpStmt}, fun.Body.List...)
									isInsertedTparallel = true
								}
							}
						}
					}
					// https://tip.golang.org/doc/go1.22
					// Loop variables are not re-initialised if the minimum version of Go is less than 1.22
					if needFixLoopVar && loopVariableUsedInRun != nil && !loopVarReInitialized {
						// insert loop var reassignment statement
						if v, ok := r.Value.(*ast.Ident); ok {
							lv := buildLoopVarReAssignmentStmt(r.Body.Lbrace, v.Name)
							r.Body.List = append([]ast.Stmt{lv}, r.Body.List...)
						}
					}
				}

				return true
			})
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

// Checks if the function has the param type *testing.T; if it does, then the
// parameter name is returned, too.
func isTestFunction(funcDecl *ast.FuncDecl) (bool, string) {
	if !strings.HasPrefix(funcDecl.Name.Name, testPrefix) {
		return false, ""
	}

	if funcDecl.Type.Params != nil && len(funcDecl.Type.Params.List) != 1 {
		return false, ""
	}

	param := funcDecl.Type.Params.List[0]

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

func hasSetenvMethod(node ast.Node, testVar string) bool {
	return exprCallHasMethod(node, testVar, "Setenv")
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

func methodSetEnvIsCalledInMethodRun(node ast.Node, testVar string) bool {
	var methodSetenvCalled bool

	if callExp, ok := node.(*ast.CallExpr); ok {
		for _, arg := range callExp.Args {
			if !methodSetenvCalled {
				ast.Inspect(arg, func(n ast.Node) bool {
					if !methodSetenvCalled {
						methodSetenvCalled = hasSetenvMethod(n, testVar)

						return true
					}

					return false
				})
			}
		}
	}

	return methodSetenvCalled
}

// build `t.Parallel()` statement to pos location specified in the argument.
func buildTParallelStmt(pos token.Pos) *ast.ExprStmt {
	return &ast.ExprStmt{
		X: &ast.CallExpr{
			Fun: &ast.SelectorExpr{
				X: &ast.Ident{
					NamePos: pos,
					Name:    "t",
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

func loopVarReferencedInRun(call *ast.CallExpr, vars []types.Object, typeInfo *types.Info) (found *string) {
	if len(call.Args) != 2 {
		return nil
	}

	ast.Inspect(call.Args[1], func(n ast.Node) bool {
		ident, ok := n.(*ast.Ident)
		if !ok {
			return true
		}
		for _, o := range vars {
			if typeInfo.ObjectOf(ident) == o {
				found = &ident.Name
			}
		}

		return true
	})

	return found
}

func loopVarReAssigned(assign *ast.AssignStmt, vars []types.Object, typeInfo *types.Info) bool {
	if len(assign.Rhs) != 1 || len(vars) != 2 {
		return false
	}

	// e.g. tt := tt
	if id, ok := assign.Rhs[0].(*ast.Ident); ok {
		if typeInfo.ObjectOf(id) == vars[1] {
			return true
		}
	}

	return false
}

func hasNolintCommentDirective(cg *ast.CommentGroup) bool {
	for _, c := range cg.List {
		if c.Text == "//nolint" || strings.Contains(c.Text, "paralleltest") || strings.Contains(c.Text, "tparallel") {
			return true
		}
	}

	return true
}

// check file top comment
// if a file has a nolint comment at the beginning, the file is removed from the target.
// also, if a specific linter (tparallel, paralleltest) is specified with a nolint comment, it is removed from the target.
func isTparagenTargetFile(fileComments []*ast.CommentGroup) bool {
	if len(fileComments) == 0 {
		return true
	}

	if fileComments[0].Pos() == 1 {
		return !hasNolintCommentDirective(fileComments[0])
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
