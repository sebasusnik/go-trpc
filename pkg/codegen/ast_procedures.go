package codegen

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"
)

// extractFromFile uses type info to extract procedures with full type resolution.
func extractFromFile(file *ast.File, info *types.Info) fileExtractionResult {
	var result fileExtractionResult

	ast.Inspect(file, func(n ast.Node) bool {
		// Detect router factory assignments: x := pkg.Func(...)
		if assign, ok := n.(*ast.AssignStmt); ok {
			extractFactoryCallsTyped(assign, info, &result)
			return true
		}

		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		// Check for r.Merge("prefix", childRouter) calls
		if merge := extractMergeCall(call); merge != nil {
			result.Merges = append(result.Merges, *merge)
			return true
		}

		// Check for gotrpc.Query() or gotrpc.Mutation() calls
		var procType string

		switch fn := call.Fun.(type) {
		case *ast.IndexListExpr:
			if sel, ok := fn.X.(*ast.SelectorExpr); ok {
				procType = getProcType(sel.Sel.Name)
			}
		case *ast.IndexExpr:
			if sel, ok := fn.X.(*ast.SelectorExpr); ok {
				procType = getProcType(sel.Sel.Name)
			}
		case *ast.SelectorExpr:
			procType = getProcType(fn.Sel.Name)
		}

		if procType == "" {
			return true
		}

		if len(call.Args) < 3 {
			return true
		}

		// Extract router variable name from first argument
		routerVar := exprToVarName(call.Args[0])

		// Extract procedure name from second argument
		nameArg, ok := call.Args[1].(*ast.BasicLit)
		if !ok || nameArg.Kind != token.STRING {
			return true
		}
		name := strings.Trim(nameArg.Value, `"`)

		// Extract handler type from third argument
		handlerExpr := call.Args[2]
		var inputType, outputType types.Type

		if info != nil {
			if tv, ok := info.Types[handlerExpr]; ok {
				inputType, outputType = extractHandlerTypes(tv.Type)
			}
		}

		if inputType == nil || outputType == nil {
			if funcLit, ok := handlerExpr.(*ast.FuncLit); ok {
				if info != nil {
					inputType, outputType = extractTypesFromFuncLit(funcLit, info)
				}
			}
		}

		result.Procedures = append(result.Procedures, ProcedureInfo{
			Name:       name,
			Type:       procType,
			InputType:  inputType,
			OutputType: outputType,
			RouterVar:  routerVar,
		})

		return true
	})

	return result
}

// extractMergeCall checks if a call expression is r.Merge("prefix", childRouter).
func extractMergeCall(call *ast.CallExpr) *MergeInfo {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Merge" {
		return nil
	}

	if len(call.Args) < 2 {
		return nil
	}

	// First arg: prefix string
	prefixArg, ok := call.Args[0].(*ast.BasicLit)
	if !ok || prefixArg.Kind != token.STRING {
		return nil
	}
	prefix := strings.Trim(prefixArg.Value, `"`)

	// Second arg: child router variable
	childVar := exprToVarName(call.Args[1])

	// Receiver: parent router variable
	parentVar := exprToVarName(sel.X)

	return &MergeInfo{
		ParentVar: parentVar,
		Prefix:    prefix,
		ChildVar:  childVar,
	}
}

// extractFromFileAST extracts procedures and merges without type info (fallback).
func extractFromFileAST(file *ast.File) fileExtractionResult {
	var result fileExtractionResult

	// Build function declaration map for resolving handler types
	funcDecls := make(map[string]*ast.FuncDecl)
	for _, decl := range file.Decls {
		if fd, ok := decl.(*ast.FuncDecl); ok && fd.Recv == nil {
			funcDecls[fd.Name.Name] = fd
		}
	}

	// Build import map for factory call detection
	imports := extractImports(file)

	ast.Inspect(file, func(n ast.Node) bool {
		// Detect router factory assignments
		if assign, ok := n.(*ast.AssignStmt); ok {
			extractFactoryCallsAST(assign, imports, &result)
			return true
		}

		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		// Check for Merge calls
		if merge := extractMergeCall(call); merge != nil {
			result.Merges = append(result.Merges, *merge)
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

		routerVar := exprToVarName(call.Args[0])

		nameArg, ok := call.Args[1].(*ast.BasicLit)
		if !ok || nameArg.Kind != token.STRING {
			return true
		}
		name := strings.Trim(nameArg.Value, `"`)

		// Try to extract input/output type names from the handler argument
		inputName, outputName := extractHandlerTypeNamesAST(call.Args[2], funcDecls)

		result.Procedures = append(result.Procedures, ProcedureInfo{
			Name:           name,
			Type:           procType,
			RouterVar:      routerVar,
			InputTypeName:  inputName,
			OutputTypeName: outputName,
		})

		return true
	})

	return result
}

// extractHandlerTypeNamesAST extracts input/output type names from a handler AST expression.
func extractHandlerTypeNamesAST(expr ast.Expr, funcDecls map[string]*ast.FuncDecl) (input, output string) {
	switch h := expr.(type) {
	case *ast.FuncLit:
		// Inline handler: func(ctx context.Context, input T) (O, error) { ... }
		return extractTypesFromFuncType(h.Type)

	case *ast.CallExpr:
		// Higher-order handler: ListTasks(store) — resolve the function's return type
		funcName := callExprFuncName(h)
		if funcName == "" {
			return "", ""
		}
		fd, ok := funcDecls[funcName]
		if !ok || fd.Type.Results == nil || len(fd.Type.Results.List) == 0 {
			return "", ""
		}
		// The return type should be a func type
		if ft, ok := fd.Type.Results.List[0].Type.(*ast.FuncType); ok {
			return extractTypesFromFuncType(ft)
		}

	case *ast.Ident:
		// Direct function reference: HealthCheck
		fd, ok := funcDecls[h.Name]
		if !ok {
			return "", ""
		}
		return extractTypesFromFuncType(fd.Type)
	}

	return "", ""
}

// callExprFuncName extracts the function name from a call expression.
func callExprFuncName(call *ast.CallExpr) string {
	switch fn := call.Fun.(type) {
	case *ast.Ident:
		return fn.Name
	case *ast.SelectorExpr:
		return fn.Sel.Name
	}
	return ""
}

// extractTypesFromFuncType extracts input/output type names from a func type AST.
// Expects: func(ctx context.Context, input I) (O, error)
func extractTypesFromFuncType(ft *ast.FuncType) (input, output string) {
	if ft == nil {
		return "", ""
	}

	// Input: second parameter
	if ft.Params != nil && len(ft.Params.List) >= 2 {
		input = exprToTypeString(ft.Params.List[1].Type)
	}

	// Output: first result (unwrap channel for subscriptions)
	if ft.Results != nil && len(ft.Results.List) >= 1 {
		resultExpr := ft.Results.List[0].Type
		// Unwrap <-chan T for subscriptions
		if chanType, ok := resultExpr.(*ast.ChanType); ok {
			resultExpr = chanType.Value
		}
		output = exprToTypeString(resultExpr)
	}

	return input, output
}

// exprToTypeString converts an AST type expression to a Go type string.
func exprToTypeString(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		if pkg, ok := e.X.(*ast.Ident); ok {
			return pkg.Name + "." + e.Sel.Name
		}
		return e.Sel.Name
	case *ast.StarExpr:
		return "*" + exprToTypeString(e.X)
	case *ast.ArrayType:
		if e.Len == nil {
			return "[]" + exprToTypeString(e.Elt)
		}
		return "[]" + exprToTypeString(e.Elt)
	case *ast.MapType:
		return "map[" + exprToTypeString(e.Key) + "]" + exprToTypeString(e.Value)
	case *ast.StructType:
		if e.Fields == nil || len(e.Fields.List) == 0 {
			return "struct{}"
		}
		var parts []string
		for _, f := range e.Fields.List {
			typeStr := exprToTypeString(f.Type)
			for _, name := range f.Names {
				tag := ""
				if f.Tag != nil {
					tag = f.Tag.Value
				}
				parts = append(parts, name.Name+":"+typeStr+":"+tag)
			}
		}
		return "struct{" + strings.Join(parts, ";") + "}"
	case *ast.InterfaceType:
		return "interface{}"
	}
	return ""
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
	// Subscription signature: func(ctx context.Context, input I) (<-chan O, error)
	if params.Len() >= 2 {
		input = params.At(1).Type()
	}
	if results.Len() >= 1 {
		output = results.At(0).Type()
		// Unwrap channel type for subscriptions
		if chanType, ok := output.(*types.Chan); ok {
			output = chanType.Elem()
		}
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
			// Unwrap channel type for subscriptions
			if chanType, ok := output.(*types.Chan); ok {
				output = chanType.Elem()
			}
		}
	}

	return input, output
}

// extractFactoryCallsTyped detects assignments like `x := pkg.Func(...)` where
// the result type is *Router, using the type checker for precise matching.
func extractFactoryCallsTyped(assign *ast.AssignStmt, info *types.Info, result *fileExtractionResult) {
	if info == nil {
		return
	}
	for i, rhs := range assign.Rhs {
		call, ok := rhs.(*ast.CallExpr)
		if !ok {
			continue
		}
		// Check if the result type is *Router
		tv, ok := info.Types[call]
		if !ok {
			continue
		}
		ptr, ok := tv.Type.(*types.Pointer)
		if !ok {
			continue
		}
		named, ok := ptr.Elem().(*types.Named)
		if !ok {
			continue
		}
		if named.Obj().Name() != "Router" {
			continue
		}
		// Must be go-trpc's Router, not some other Router
		if named.Obj().Pkg() == nil || !strings.Contains(named.Obj().Pkg().Path(), "go-trpc") {
			continue
		}
		if i >= len(assign.Lhs) {
			continue
		}
		localVar := exprToVarName(assign.Lhs[i])
		if localVar == "" {
			continue
		}
		// Get the factory's package path from the call expression
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			continue // local call (e.g., gotrpc.NewRouter()), skip
		}
		pkgIdent, ok := sel.X.(*ast.Ident)
		if !ok {
			continue
		}
		use, ok := info.Uses[pkgIdent]
		if !ok {
			continue
		}
		pkgName, ok := use.(*types.PkgName)
		if !ok {
			continue
		}
		factoryPkg := pkgName.Imported().Path()
		// Skip go-trpc itself (gotrpc.NewRouter() is not a factory)
		if strings.Contains(factoryPkg, "go-trpc") {
			continue
		}
		result.FactoryCalls = append(result.FactoryCalls, RouterFactoryCall{
			LocalVar:       localVar,
			FactoryPkgPath: factoryPkg,
		})
	}
}

// extractFactoryCallsAST detects router factory assignments without type info.
// Uses heuristics: looks for `x := pkg.SomeFunc(...)` where pkg is an imported package.
func extractFactoryCallsAST(assign *ast.AssignStmt, imports map[string]string, result *fileExtractionResult) {
	for i, rhs := range assign.Rhs {
		call, ok := rhs.(*ast.CallExpr)
		if !ok {
			continue
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			continue
		}
		pkgIdent, ok := sel.X.(*ast.Ident)
		if !ok {
			continue
		}
		// Look up the import path for this alias
		importPath, ok := imports[pkgIdent.Name]
		if !ok {
			continue
		}
		// Skip go-trpc itself
		if strings.Contains(importPath, "go-trpc") {
			continue
		}
		if i >= len(assign.Lhs) {
			continue
		}
		localVar := exprToVarName(assign.Lhs[i])
		if localVar == "" {
			continue
		}
		// Use the package name (last segment of import path) as the factory pkg path
		// to match against procedures' PkgPath (which is set to pkg.Name in ParseDir)
		parts := strings.Split(importPath, "/")
		pkgName := parts[len(parts)-1]
		result.FactoryCalls = append(result.FactoryCalls, RouterFactoryCall{
			LocalVar:       localVar,
			FactoryPkgPath: pkgName,
		})
	}
}

// extractImports builds a map from import alias to import path for a file.
func extractImports(file *ast.File) map[string]string {
	imports := make(map[string]string)
	for _, imp := range file.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		var alias string
		if imp.Name != nil {
			alias = imp.Name.Name
		} else {
			// Default alias is the last segment of the import path
			parts := strings.Split(path, "/")
			alias = parts[len(parts)-1]
		}
		imports[alias] = path
	}
	return imports
}
