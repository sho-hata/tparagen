package tparagen

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
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

	ast.Inspect(f, func(n ast.Node) bool {
		if funcDecl, ok := n.(*ast.FuncDecl); ok {
			var (
				funcHasParallelMethod bool
				numberOfTestRun       int
				positionOfTestRunNode []ast.Node
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

							hasParallel := false
							numberOfTestRun++
							ast.Inspect(v, func(p ast.Node) bool {
								if !hasParallel {
									hasParallel = methodParallelIsCalledInTestFunction(p, innerTestVar)
								}

								return true
							})
							if !hasParallel {
								// t.Run でparallelが呼び出されていなかったとき
								positionOfTestRunNode = append(positionOfTestRunNode, n)
							}
						}

						return true
					})
				}
			}
			if !funcHasParallelMethod {
				ds := buildTParallelStmt(funcDecl.Body.Lbrace)
				funcDecl.Body.List = append([]ast.Stmt{ds}, funcDecl.Body.List...)
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

func buildTParallelStmt(pos token.Pos) *ast.ExprStmt {
	return &ast.ExprStmt{
		X: &ast.CallExpr{
			Fun: &ast.SelectorExpr{
				X: &ast.Ident{
					NamePos: pos,
					Name:    "t",
					Obj:     nil, // TODO: ?
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
