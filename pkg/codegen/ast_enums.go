package codegen

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/packages"
)

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
