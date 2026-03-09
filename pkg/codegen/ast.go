package codegen

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
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
	RouterVar  string // variable name of the router this is registered on

	// AST-only fields (used when type checking is unavailable)
	InputTypeName  string // Go type expression as string (e.g., "ListTasksInput")
	OutputTypeName string // Go type expression as string (e.g., "Task")
}

// MergeInfo holds information about a router Merge() call.
type MergeInfo struct {
	ParentVar string // the router being merged into (e.g., "r")
	Prefix    string // the namespace prefix (e.g., "task")
	ChildVar  string // the router being merged (e.g., "taskRouter")
}

// EnumInfo holds information about a Go enum pattern (type X string/int + const block).
type EnumInfo struct {
	TypeName      string   // the Go type short name (e.g., "Status")
	QualifiedName string   // fully qualified name (e.g., "example.com/models.Status")
	Values        []string // the const values (e.g., ["active", "inactive"])
	IsString      bool     // true if underlying type is string, false if int
}

// StructField holds information about a struct field extracted from AST.
type StructField struct {
	Name     string
	TypeExpr string // Go type expression as string (e.g., "string", "[]Task")
	JSONName string
	Optional bool // from omitempty tag
}

// StructDef holds information about a struct type extracted from AST.
type StructDef struct {
	Name   string
	Fields []StructField
}

// ParseResult holds the result of parsing Go source for tRPC procedures.
type ParseResult struct {
	Procedures []ProcedureInfo
	Merges     []MergeInfo
	Enums      []EnumInfo
	StructDefs []StructDef
	RouterVar  string // name of the router variable (e.g., "appRouter")
}

// ParsePackage parses Go source files in the given directory pattern and extracts tRPC procedure registrations.
func ParsePackage(pattern string, routerVar string) (*ParseResult, error) {
	absPattern, err := filepath.Abs(pattern)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve path: %w", err)
	}

	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax |
			packages.NeedTypes | packages.NeedTypesInfo,
		Dir: absPattern,
	}

	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		return nil, fmt.Errorf("failed to load packages: %w", err)
	}

	if len(pkgs) == 0 {
		return nil, fmt.Errorf("no packages found in %s", pattern)
	}

	result := &ParseResult{RouterVar: routerVar}

	for _, pkg := range pkgs {
		if len(pkg.Errors) > 0 {
			for _, e := range pkg.Errors {
				fmt.Fprintf(os.Stderr, "warning: %s\n", e)
			}
		}

		for _, file := range pkg.Syntax {
			extracted := extractFromFile(file, pkg.TypesInfo)
			result.Procedures = append(result.Procedures, extracted.Procedures...)
			result.Merges = append(result.Merges, extracted.Merges...)
		}

		// Extract enum patterns from the package
		result.Enums = append(result.Enums, extractEnums(pkg)...)
	}

	resolvePrefixes(result)
	return result, nil
}

// ParseDir parses Go source files using the simpler go/parser approach (no type checking).
// This is a fallback when golang.org/x/tools/go/packages is not available.
// It recursively scans subdirectories.
func ParseDir(dir string) (*ParseResult, error) {
	result := &ParseResult{}

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return nil
		}

		fset := token.NewFileSet()
		pkgs, err := parser.ParseDir(fset, path, nil, parser.ParseComments)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to parse %s: %v\n", path, err)
			return nil
		}

		for _, pkg := range pkgs {
			for _, file := range pkg.Files {
				extracted := extractFromFileAST(file)
				result.Procedures = append(result.Procedures, extracted.Procedures...)
				result.Merges = append(result.Merges, extracted.Merges...)
				result.Enums = append(result.Enums, extractEnumsFromAST(file)...)
				result.StructDefs = append(result.StructDefs, extractStructDefsFromAST(file)...)
			}
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to walk directory %s: %w", dir, err)
	}

	resolvePrefixes(result)
	return result, nil
}

// fileExtractionResult holds procedures and merges extracted from a single file.
type fileExtractionResult struct {
	Procedures []ProcedureInfo
	Merges     []MergeInfo
}

// extractProcedures uses type info to extract procedures with full type resolution.
func extractFromFile(file *ast.File, info *types.Info) fileExtractionResult {
	var result fileExtractionResult

	ast.Inspect(file, func(n ast.Node) bool {
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

// exprToVarName extracts a variable name from an expression.
func exprToVarName(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	default:
		return ""
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

	ast.Inspect(file, func(n ast.Node) bool {
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

// extractStructDefsFromAST extracts struct type definitions from a Go AST file.
func extractStructDefsFromAST(file *ast.File) []StructDef {
	var defs []StructDef

	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}

		for _, spec := range genDecl.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}

			st, ok := ts.Type.(*ast.StructType)
			if !ok {
				continue
			}

			def := StructDef{Name: ts.Name.Name}
			if st.Fields != nil {
				for _, field := range st.Fields.List {
					typeStr := exprToTypeString(field.Type)
					jsonName := ""
					optional := false
					if field.Tag != nil {
						tag := strings.Trim(field.Tag.Value, "`")
						jsonName, optional = parseJSONTagFromString(tag)
					}

					for _, name := range field.Names {
						fn := jsonName
						if fn == "" {
							fn = name.Name
						}
						def.Fields = append(def.Fields, StructField{
							Name:     name.Name,
							TypeExpr: typeStr,
							JSONName: fn,
							Optional: optional,
						})
					}
				}
			}
			defs = append(defs, def)
		}
	}

	return defs
}

// parseJSONTagFromString extracts JSON field name and omitempty from a struct tag string.
func parseJSONTagFromString(tag string) (name string, optional bool) {
	// Find json:"..." in the tag
	const prefix = `json:"`
	idx := strings.Index(tag, prefix)
	if idx < 0 {
		return "", false
	}
	rest := tag[idx+len(prefix):]
	end := strings.Index(rest, `"`)
	if end < 0 {
		return "", false
	}
	jsonVal := rest[:end]
	if jsonVal == "-" {
		return "-", false
	}
	parts := strings.Split(jsonVal, ",")
	name = parts[0]
	for _, p := range parts[1:] {
		if p == "omitempty" {
			optional = true
		}
	}
	return name, optional
}

// resolvePrefixes applies Merge() namespace prefixes to procedure names.
// It builds a prefix chain from Merge calls and prepends to each procedure name.
func resolvePrefixes(result *ParseResult) {
	if len(result.Merges) == 0 {
		return
	}

	// Build map: childVar -> (parentVar, prefix)
	// A child router may be merged into a parent with a prefix.
	type mergeEntry struct {
		parentVar string
		prefix    string
	}
	mergeMap := make(map[string]mergeEntry)
	for _, m := range result.Merges {
		mergeMap[m.ChildVar] = mergeEntry{parentVar: m.ParentVar, prefix: m.Prefix}
	}

	// For each procedure, walk up the merge chain to build the full prefix.
	for i, proc := range result.Procedures {
		if proc.RouterVar == "" {
			continue
		}

		var prefixes []string
		current := proc.RouterVar
		seen := make(map[string]bool) // prevent cycles

		for {
			if seen[current] {
				break
			}
			seen[current] = true

			entry, ok := mergeMap[current]
			if !ok {
				break
			}
			prefixes = append(prefixes, entry.prefix)
			current = entry.parentVar
		}

		if len(prefixes) > 0 {
			// Reverse prefixes (we collected child->parent, need parent->child)
			for l, r := 0, len(prefixes)-1; l < r; l, r = l+1, r-1 {
				prefixes[l], prefixes[r] = prefixes[r], prefixes[l]
			}
			result.Procedures[i].Name = strings.Join(prefixes, ".") + "." + proc.Name
		}
	}
}

func getProcType(name string) string {
	switch name {
	case "Query":
		return "query"
	case "Mutation":
		return "mutation"
	case "Subscription":
		return "subscription"
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

// extractEnums detects Go enum patterns: named types with string/int underlying + const blocks.
func extractEnums(pkg *packages.Package) []EnumInfo {
	var enums []EnumInfo

	// Find all named types with string or int underlying types
	scope := pkg.Types.Scope()
	for _, name := range scope.Names() {
		obj := scope.Lookup(name)
		tn, ok := obj.(*types.TypeName)
		if !ok {
			continue
		}

		named, ok := tn.Type().(*types.Named)
		if !ok {
			continue
		}

		basic, ok := named.Underlying().(*types.Basic)
		if !ok {
			continue
		}

		isString := basic.Kind() == types.String
		isInt := basic.Kind() == types.Int || basic.Kind() == types.Int8 ||
			basic.Kind() == types.Int16 || basic.Kind() == types.Int32 ||
			basic.Kind() == types.Int64 || basic.Kind() == types.Uint ||
			basic.Kind() == types.Uint8 || basic.Kind() == types.Uint16 ||
			basic.Kind() == types.Uint32 || basic.Kind() == types.Uint64

		if !isString && !isInt {
			continue
		}

		// Find const values of this type
		var values []string
		for _, constName := range scope.Names() {
			constObj := scope.Lookup(constName)
			c, ok := constObj.(*types.Const)
			if !ok {
				continue
			}

			// Check if the const is of the named type
			if !types.Identical(c.Type(), named) {
				continue
			}

			// Extract the value
			val := c.Val().ExactString()
			if isString {
				// Remove surrounding quotes if present
				val = strings.Trim(val, `"`)
			}
			values = append(values, val)
		}

		if len(values) > 0 {
			qualName := name
			if tn.Pkg() != nil {
				qualName = tn.Pkg().Path() + "." + name
			}
			enums = append(enums, EnumInfo{
				TypeName:      name,
				QualifiedName: qualName,
				Values:        values,
				IsString:      isString,
			})
		}
	}

	return enums
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

// extractEnumsFromAST detects enum patterns from AST without type checking.
// Supports string enums (explicit string literals) and simple iota enums.
func extractEnumsFromAST(file *ast.File) []EnumInfo {
	var enums []EnumInfo

	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.CONST {
			continue
		}

		// Group const specs by their declared type
		type constEntry struct {
			name  string
			value string
		}
		var typeName string
		var entries []constEntry
		hasIota := false
		isString := false

		for i, spec := range genDecl.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}

			// Determine the type name from the first spec that has one
			if vs.Type != nil {
				if ident, ok := vs.Type.(*ast.Ident); ok {
					if typeName == "" {
						typeName = ident.Name
					} else if typeName != ident.Name {
						// Mixed types in const block — skip
						typeName = ""
						break
					}
				}
			}

			for j, name := range vs.Names {
				if name.Name == "_" {
					continue
				}

				var val string
				// Check for explicit values
				if j < len(vs.Values) {
					switch v := vs.Values[j].(type) {
					case *ast.BasicLit:
						val = strings.Trim(v.Value, `"`)
						if v.Kind == token.STRING {
							isString = true
						}
					case *ast.Ident:
						if v.Name == "iota" {
							hasIota = true
							val = fmt.Sprintf("%d", i)
						}
					}
				} else if hasIota {
					// Implicit iota continuation
					val = fmt.Sprintf("%d", i)
				}

				if val != "" {
					entries = append(entries, constEntry{name: name.Name, value: val})
				}
			}
		}

		if typeName == "" || len(entries) == 0 {
			continue
		}

		values := make([]string, len(entries))
		for i, e := range entries {
			values[i] = e.value
		}

		enums = append(enums, EnumInfo{
			TypeName:      typeName,
			QualifiedName: typeName,
			Values:        values,
			IsString:      isString,
		})
	}

	return enums
}
