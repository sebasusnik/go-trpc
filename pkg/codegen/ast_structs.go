package codegen

import (
	"go/ast"
	"go/token"
	"strings"
)

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
