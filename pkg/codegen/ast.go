package codegen

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/packages"
)

// ProcedureInfo holds information about a discovered tRPC procedure.
type ProcedureInfo struct {
	Name       string
	Type       string // "query" or "mutation"
	InputType  types.Type
	OutputType types.Type
}

// ParseResult holds the result of parsing Go source for tRPC procedures.
type ParseResult struct {
	Procedures []ProcedureInfo
	RouterVar  string // name of the router variable (e.g., "appRouter")
}

// ParsePackage parses Go source files in the given directory pattern and extracts tRPC procedure registrations.
func ParsePackage(pattern string, routerVar string) (*ParseResult, error) {
	// Resolve pattern to absolute path
	absPattern, err := filepath.Abs(pattern)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve path: %w", err)
	}

	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax |
			packages.NeedTypes | packages.NeedTypesInfo,
		Dir: absPattern,
	}

	pkgs, err := packages.Load(cfg, ".")
	if err != nil {
		return nil, fmt.Errorf("failed to load packages: %w", err)
	}

	if len(pkgs) == 0 {
		return nil, fmt.Errorf("no packages found in %s", pattern)
	}

	result := &ParseResult{RouterVar: routerVar}

	for _, pkg := range pkgs {
		if len(pkg.Errors) > 0 {
			// Collect errors but continue
			for _, e := range pkg.Errors {
				fmt.Printf("warning: %s\n", e)
			}
		}

		for _, file := range pkg.Syntax {
			procedures := extractProcedures(file, pkg.TypesInfo)
			result.Procedures = append(result.Procedures, procedures...)
		}
	}

	return result, nil
}

// ParseDir parses Go source files using the simpler go/parser approach (no type checking).
// This is a fallback when golang.org/x/tools/go/packages is not available.
func ParseDir(dir string) (*ParseResult, error) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, nil, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("failed to parse directory %s: %w", dir, err)
	}

	result := &ParseResult{}

	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			procedures := extractProceduresFromAST(file)
			result.Procedures = append(result.Procedures, procedures...)
		}
	}

	return result, nil
}

// extractProcedures uses type info to extract procedures with full type resolution.
func extractProcedures(file *ast.File, info *types.Info) []ProcedureInfo {
	var procedures []ProcedureInfo

	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		// Check for gotrpc.Query() or gotrpc.Mutation() calls
		// Can be either:
		//   - gotrpc.Query(r, "name", handler)
		//   - router.Query(r, "name", handler)  -- package-level function
		var procType string

		switch fn := call.Fun.(type) {
		case *ast.IndexListExpr:
			// Generic call: gotrpc.Query[I, O](r, "name", handler)
			if sel, ok := fn.X.(*ast.SelectorExpr); ok {
				procType = getProcType(sel.Sel.Name)
			}
		case *ast.IndexExpr:
			// Single type param generic call
			if sel, ok := fn.X.(*ast.SelectorExpr); ok {
				procType = getProcType(sel.Sel.Name)
			}
		case *ast.SelectorExpr:
			procType = getProcType(fn.Sel.Name)
		}

		if procType == "" {
			return true
		}

		// Need at least 3 args: router, name, handler
		if len(call.Args) < 3 {
			return true
		}

		// Extract procedure name from second argument (string literal)
		nameArg, ok := call.Args[1].(*ast.BasicLit)
		if !ok || nameArg.Kind != token.STRING {
			return true
		}
		name := strings.Trim(nameArg.Value, `"`)

		// Extract handler type from third argument
		handlerExpr := call.Args[2]
		var inputType, outputType types.Type

		if info != nil {
			// Try to get type from type info
			if tv, ok := info.Types[handlerExpr]; ok {
				inputType, outputType = extractHandlerTypes(tv.Type)
			}
		}

		// If type info didn't work, try to extract from AST
		if inputType == nil || outputType == nil {
			if funcLit, ok := handlerExpr.(*ast.FuncLit); ok {
				if info != nil {
					inputType, outputType = extractTypesFromFuncLit(funcLit, info)
				}
			}
		}

		procedures = append(procedures, ProcedureInfo{
			Name:       name,
			Type:       procType,
			InputType:  inputType,
			OutputType: outputType,
		})

		return true
	})

	return procedures
}

// extractProceduresFromAST extracts procedures without type info (fallback).
func extractProceduresFromAST(file *ast.File) []ProcedureInfo {
	var procedures []ProcedureInfo

	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		var procType string
		switch fn := call.Fun.(type) {
		case *ast.SelectorExpr:
			procType = getProcType(fn.Sel.Name)
		case *ast.IndexListExpr:
			if sel, ok := fn.X.(*ast.SelectorExpr); ok {
				procType = getProcType(sel.Sel.Name)
			}
		case *ast.IndexExpr:
			if sel, ok := fn.X.(*ast.SelectorExpr); ok {
				procType = getProcType(sel.Sel.Name)
			}
		}

		if procType == "" || len(call.Args) < 3 {
			return true
		}

		nameArg, ok := call.Args[1].(*ast.BasicLit)
		if !ok || nameArg.Kind != token.STRING {
			return true
		}
		name := strings.Trim(nameArg.Value, `"`)

		procedures = append(procedures, ProcedureInfo{
			Name: name,
			Type: procType,
		})

		return true
	})

	return procedures
}

func getProcType(name string) string {
	switch name {
	case "Query":
		return "query"
	case "Mutation":
		return "mutation"
	default:
		return ""
	}
}

// extractHandlerTypes extracts input and output types from a handler function type.
func extractHandlerTypes(t types.Type) (input, output types.Type) {
	sig, ok := t.(*types.Signature)
	if !ok {
		return nil, nil
	}

	params := sig.Params()
	results := sig.Results()

	// Handler signature: func(ctx context.Context, input I) (O, error)
	if params.Len() >= 2 {
		input = params.At(1).Type()
	}
	if results.Len() >= 1 {
		output = results.At(0).Type()
	}

	return input, output
}

// extractTypesFromFuncLit extracts types from a function literal's signature.
func extractTypesFromFuncLit(fn *ast.FuncLit, info *types.Info) (input, output types.Type) {
	if fn.Type == nil {
		return nil, nil
	}

	// Get input type from second parameter
	if fn.Type.Params != nil && len(fn.Type.Params.List) >= 2 {
		paramField := fn.Type.Params.List[1]
		if tv, ok := info.Types[paramField.Type]; ok {
			input = tv.Type
		}
	}

	// Get output type from first result
	if fn.Type.Results != nil && len(fn.Type.Results.List) >= 1 {
		resultField := fn.Type.Results.List[0]
		if tv, ok := info.Types[resultField.Type]; ok {
			output = tv.Type
		}
	}

	return input, output
}
