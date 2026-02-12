package setup

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInitializeDependencies_WiresWorkflowCELOptionsProvider(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "setup.go", nil, 0)
	require.NoError(t, err)

	foundWorkflowsvcNew := false
	foundCELOptionsProvider := false

	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if sel.Sel == nil || sel.Sel.Name != "New" {
			return true
		}
		pkg, ok := sel.X.(*ast.Ident)
		if !ok || pkg.Name != "workflowsvc" {
			return true
		}
		if len(call.Args) == 0 {
			return true
		}
		foundWorkflowsvcNew = true

		lit, ok := call.Args[0].(*ast.CompositeLit)
		if !ok {
			return true
		}
		for _, elt := range lit.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			key, ok := kv.Key.(*ast.Ident)
			if !ok {
				continue
			}
			if key.Name == "CELOptionsProvider" {
				foundCELOptionsProvider = true
				return false
			}
		}
		return true
	})

	require.True(t, foundWorkflowsvcNew, "expected setup.InitializeDependencies to call workflowsvc.New")
	require.True(t, foundCELOptionsProvider, "expected workflowsvc.New config to set CELOptionsProvider")
}

