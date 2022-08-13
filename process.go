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

func Process(filename string, src []byte) ([]byte, error) {
	fs := token.NewFileSet()

	f, err := parser.ParseFile(fs, filename, src, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("cannot pase file. %w", err)
	}

	typesInfo := &types.Info{Defs: map[*ast.Ident]types.Object{}}

	ast.Inspect(f, func(n ast.Node) bool {
		if funcDecl, ok := n.(*ast.FuncDecl); ok {
			var (
				funcHasParallelMethod,
				rangeStatementOverTestCasesExists,
				rangeStatementHasParallelMethod,
				loopVarReInitialised bool

				loopVariableUsedInRun *string

				rangeNode ast.Node
			)

			// Check runs for test functions only
			isTest, testVar := isTestFunction(funcDecl)
			if !isTest {
				return true
			}

			for _, l := range funcDecl.Body.List {
				switch v := l.(type) {
				case *ast.ExprStmt:
					ast.Inspect(v, func(n ast.Node) bool {
						// Check if the test method is calling t.Parallel
						if !funcHasParallelMethod {
							funcHasParallelMethod = methodParallelIsCalledInTestFunction(n, testVar)
						}

						// Check if the t.Run within the test function is calling t.Parallel
						if methodRunIsCalledInTestFunction(n, testVar) {
							// n is a call to t.Run; find out the name of the subtest's *testing.T parameter.
							innerTestVar := getRunCallbackParameterName(n)
							if innerTestVar == "" {
								return true
							}

							hasParallel := false

							ast.Inspect(v, func(p ast.Node) bool {
								if !hasParallel {
									hasParallel = methodParallelIsCalledInTestFunction(p, innerTestVar)
								}

								return true
							})

							if !hasParallel {
								if n, ok := n.(*ast.CallExpr); ok {
									funcArg := n.Args[1]

									if fun, ok := funcArg.(*ast.FuncLit); ok {
										tpStmt := buildTParallelStmt(fun.Body.Lbrace)
										fun.Body.List = append([]ast.Stmt{tpStmt}, fun.Body.List...)
									}
								}
							}
						}

						return true
					})

					// Check if the range over testcases is calling t.Parallel
				case *ast.RangeStmt:
					rangeNode = v

					var loopVars []types.Object
					for _, expr := range []ast.Expr{v.Key, v.Value} {
						if id, ok := expr.(*ast.Ident); ok {
							loopVars = append(loopVars, typesInfo.ObjectOf(id))
						}
					}

					ast.Inspect(v, func(n ast.Node) bool {
						switch n := n.(type) {
						case *ast.ExprStmt:
							if methodRunIsCalledInRangeStatement(n.X, testVar) {
								// e.X is a call to t.Run; find out the name of the subtest's *testing.T parameter.
								innerTestVar := getRunCallbackParameterName(n.X)

								rangeStatementOverTestCasesExists = true

								if !rangeStatementHasParallelMethod {
									rangeStatementHasParallelMethod = methodParallelIsCalledInMethodRun(n.X, innerTestVar)
								}

								if loopVariableUsedInRun == nil {
									if run, ok := n.X.(*ast.CallExpr); ok {
										loopVariableUsedInRun = loopVarReferencedInRun(run, loopVars, typesInfo)
									}
								}
							}
						case *ast.AssignStmt:
							if !loopVarReInitialised {
								loopVarReInitialised = loopVarReAssigned(n, loopVars, typesInfo)
							}
						}

						return true
					})
				}
			}

			// Check if the main test calls t.Parallel.
			if !funcHasParallelMethod {
				tpStmt := buildTParallelStmt(funcDecl.Body.Lbrace)
				funcDecl.Body.List = append([]ast.Stmt{tpStmt}, funcDecl.Body.List...)
			}

			// Check if the sub tests calls t.Parallel.
			if rangeNode != nil && rangeStatementOverTestCasesExists && !rangeStatementHasParallelMethod {
				var isInsertedTparallel bool

				ast.Inspect(rangeNode, func(n ast.Node) bool {
					if isInsertedTparallel {
						return true
					}

					if r, ok := rangeNode.(*ast.RangeStmt); ok {
						for _, n := range r.Body.List {
							if e, ok := n.(*ast.ExprStmt); ok {
								if !methodRunIsCalledInRangeStatement(e.X, testVar) {
									continue
								}

								if c, ok := e.X.(*ast.CallExpr); ok {
									if len(c.Args) != 2 {
										return true
									}
									funcArg := c.Args[1]

									if fun, ok := funcArg.(*ast.FuncLit); ok {
										tpStmt := buildTParallelStmt(fun.Body.Lbrace)
										fun.Body.List = append([]ast.Stmt{tpStmt}, fun.Body.List...)
										isInsertedTparallel = true
									}
								}
							}
						}
					}

					return true
				})

				if loopVariableUsedInRun != nil && !loopVarReInitialised {
					if r, ok := rangeNode.(*ast.RangeStmt); ok {
						if v, ok := r.Value.(*ast.Ident); ok {
							lv := buildLoopVarReAssignmentStmt(r.Body.Lbrace, v.Name)
							r.Body.List = append([]ast.Stmt{lv}, r.Body.List...)
						}
					}
				}
			}
		}

		return true
	})

	// gofmt
	var fmtedBuf bytes.Buffer
	if err := format.Node(&fmtedBuf, fs, f); err != nil {
		return nil, err
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
	if starExp, ok := param.Type.(*ast.StarExpr); ok {
		if selectExpr, ok := starExp.X.(*ast.SelectorExpr); ok {
			if selectExpr.Sel.Name == testMethodStruct {
				if s, ok := selectExpr.X.(*ast.Ident); ok {
					return s.Name == testMethodPackageType, param.Names[0].Name
				}
			}
		}
	}

	return false, ""
}

func exprCallHasMethod(node ast.Node, receiverName, methodName string) bool {
	switch n := node.(type) {
	case *ast.CallExpr:
		if fun, ok := n.Fun.(*ast.SelectorExpr); ok {
			if receiver, ok := fun.X.(*ast.Ident); ok {
				return receiver.Name == receiverName && fun.Sel.Name == methodName
			}
		}
	}

	return false
}

func methodParallelIsCalledInTestFunction(node ast.Node, testVar string) bool {
	return exprCallHasMethod(node, testVar, "Parallel")
}

func methodRunIsCalledInTestFunction(node ast.Node, testVar string) bool {
	return exprCallHasMethod(node, testVar, "Run")
}

func methodRunIsCalledInRangeStatement(node ast.Node, testVar string) bool {
	return exprCallHasMethod(node, testVar, "Run")
}

func methodParallelIsCalledInRunMethod(node ast.Node, testVar string) bool {
	return exprCallHasMethod(node, testVar, "Parallel")
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
	var methodParallelCalled bool
	// nolint: gocritic
	switch callExp := node.(type) {
	case *ast.CallExpr:
		for _, arg := range callExp.Args {
			if !methodParallelCalled {
				ast.Inspect(arg, func(n ast.Node) bool {
					if !methodParallelCalled {
						methodParallelCalled = methodParallelIsCalledInRunMethod(n, testVar)

						return true
					}

					return false
				})
			}
		}
	}

	return methodParallelCalled
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
				Obj: &ast.Object{
					Name: varName,
					Type: testMethodPackageType,
				},
			},
		},
		TokPos: pos,
		Tok:    token.DEFINE,
		Rhs: []ast.Expr{
			&ast.Ident{
				NamePos: pos,
				Name:    varName,
				Obj: &ast.Object{
					Name: varName,
					Type: testMethodPackageType,
				},
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
