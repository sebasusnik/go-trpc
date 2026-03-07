package codegen

import (
	"fmt"
	"go/types"
	"strings"
)

// TSType represents a generated TypeScript type.
type TSType struct {
	Name       string
	Definition string
	Inline     string // for inline usage when no named type exists
}

// TypeMapper converts Go types to TypeScript type representations.
type TypeMapper struct {
	namedTypes map[string]*TSType // collected named type definitions
}

// NewTypeMapper creates a new TypeMapper.
func NewTypeMapper() *TypeMapper {
	return &TypeMapper{
		namedTypes: make(map[string]*TSType),
	}
}

// NamedTypes returns all collected named type definitions.
func (m *TypeMapper) NamedTypes() map[string]*TSType {
	return m.namedTypes
}

// MapType maps a Go type to its TypeScript representation.
// Returns the TypeScript type string to use in references.
func (m *TypeMapper) MapType(t types.Type) string {
	return m.mapType(t, true)
}

func (m *TypeMapper) mapType(t types.Type, topLevel bool) string {
	switch typ := t.(type) {
	case *types.Named:
		obj := typ.Obj()
		name := obj.Name()

		// Check for well-known types
		if obj.Pkg() != nil {
			fullName := obj.Pkg().Path() + "." + name
			if fullName == "time.Time" {
				return "string"
			}
		}

		// If it's a struct, register and return by name
		underlying := typ.Underlying()
		if _, ok := underlying.(*types.Struct); ok {
			if _, exists := m.namedTypes[name]; !exists {
				// Placeholder to prevent infinite recursion
				m.namedTypes[name] = &TSType{Name: name}
				def := m.mapStructType(underlying.(*types.Struct))
				m.namedTypes[name].Definition = def
			}
			return name
		}

		// For non-struct named types, map the underlying type
		return m.mapType(underlying, false)

	case *types.Basic:
		return mapBasicType(typ)

	case *types.Pointer:
		elem := m.mapType(typ.Elem(), false)
		return elem + " | null"

	case *types.Slice:
		// []byte -> string (base64)
		if basic, ok := typ.Elem().(*types.Basic); ok && basic.Kind() == types.Byte {
			return "string"
		}
		elem := m.mapType(typ.Elem(), false)
		return elem + "[]"

	case *types.Array:
		elem := m.mapType(typ.Elem(), false)
		return elem + "[]"

	case *types.Map:
		val := m.mapType(typ.Elem(), false)
		return fmt.Sprintf("Record<string, %s>", val)

	case *types.Struct:
		return m.mapStructType(typ)

	case *types.Interface:
		return "unknown"

	default:
		return "unknown"
	}
}

func (m *TypeMapper) mapStructType(s *types.Struct) string {
	if s.NumFields() == 0 {
		return "Record<string, never>"
	}

	var fields []string
	for i := 0; i < s.NumFields(); i++ {
		field := s.Field(i)
		if !field.Exported() {
			continue
		}

		tag := s.Tag(i)
		jsonName, opts := parseJSONTag(tag)

		// Skip fields with json:"-"
		if jsonName == "-" {
			continue
		}

		// Use json tag name or field name (camelCase)
		name := jsonName
		if name == "" {
			name = lowerFirst(field.Name())
		}

		tsType := m.mapType(field.Type(), false)
		optional := ""
		if opts.omitempty {
			optional = "?"
		}

		fields = append(fields, fmt.Sprintf("  %s%s: %s;", name, optional, tsType))
	}

	return "{\n" + strings.Join(fields, "\n") + "\n}"
}

func mapBasicType(b *types.Basic) string {
	switch b.Kind() {
	case types.String:
		return "string"
	case types.Bool:
		return "boolean"
	case types.Int, types.Int8, types.Int16, types.Int32, types.Int64,
		types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64,
		types.Float32, types.Float64:
		return "number"
	default:
		return "unknown"
	}
}

type jsonTagOpts struct {
	omitempty bool
}

func parseJSONTag(tag string) (string, jsonTagOpts) {
	// Parse struct tag to find json tag
	var opts jsonTagOpts

	// Simple struct tag parser
	jsonTag := ""
	for tag != "" {
		// Skip whitespace
		i := 0
		for i < len(tag) && tag[i] == ' ' {
			i++
		}
		tag = tag[i:]
		if tag == "" {
			break
		}

		// Find key
		i = 0
		for i < len(tag) && tag[i] > ' ' && tag[i] != ':' && tag[i] != '"' {
			i++
		}
		key := tag[:i]
		tag = tag[i:]

		if len(tag) < 2 || tag[0] != ':' || tag[1] != '"' {
			break
		}
		tag = tag[2:]

		// Find value end
		i = 0
		for i < len(tag) && tag[i] != '"' {
			i++
		}
		val := tag[:i]
		if i < len(tag) {
			tag = tag[i+1:]
		} else {
			tag = ""
		}

		if key == "json" {
			jsonTag = val
			break
		}
	}

	if jsonTag == "" {
		return "", opts
	}

	parts := strings.Split(jsonTag, ",")
	name := parts[0]
	for _, p := range parts[1:] {
		if p == "omitempty" {
			opts.omitempty = true
		}
	}
	return name, opts
}

func lowerFirst(s string) string {
	if len(s) == 0 {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}
