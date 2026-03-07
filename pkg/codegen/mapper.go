package codegen

import (
	"fmt"
	"go/types"
	"strings"
	"unicode"
)

// TSType represents a generated TypeScript type.
type TSType struct {
	Name       string
	Definition string
	Inline     string // for inline usage when no named type exists
}

// wellKnownTypes maps fully qualified Go type names to their TypeScript equivalents.
var wellKnownTypes = map[string]string{
	"time.Time":                                 "string",
	"github.com/google/uuid.UUID":               "string",
	"encoding/json.RawMessage":                  "unknown",
	"database/sql.NullString":                   "string | null",
	"database/sql.NullInt64":                    "number | null",
	"database/sql.NullInt32":                    "number | null",
	"database/sql.NullInt16":                    "number | null",
	"database/sql.NullFloat64":                  "number | null",
	"database/sql.NullBool":                     "boolean | null",
	"database/sql.NullTime":                     "string | null",
	"database/sql.NullByte":                     "number | null",
	"github.com/shopspring/decimal.Decimal":     "string",
	"github.com/shopspring/decimal.NullDecimal": "string | null",
}

// TypeMapper converts Go types to TypeScript type representations.
type TypeMapper struct {
	// namedTypes stores type definitions keyed by qualified name (pkg/path.TypeName)
	namedTypes map[string]*TSType
	// tsNames maps qualified name -> TypeScript name to use in output
	tsNames map[string]string
	// shortNameOwners maps short name -> qualified name (for collision detection)
	shortNameOwners map[string]string
	// enumTypes maps Go type name -> enum info for union type generation
	enumTypes map[string]*EnumInfo
}

// NewTypeMapper creates a new TypeMapper.
func NewTypeMapper() *TypeMapper {
	return &TypeMapper{
		namedTypes:      make(map[string]*TSType),
		tsNames:         make(map[string]string),
		shortNameOwners: make(map[string]string),
		enumTypes:       make(map[string]*EnumInfo),
	}
}

// RegisterEnums registers detected enum patterns so the mapper can generate union types.
func (m *TypeMapper) RegisterEnums(enums []EnumInfo) {
	for i := range enums {
		key := enums[i].QualifiedName
		if key == "" {
			key = enums[i].TypeName
		}
		m.enumTypes[key] = &enums[i]
	}
}

// NamedTypes returns all collected named type definitions, keyed by their TypeScript name.
func (m *TypeMapper) NamedTypes() map[string]*TSType {
	result := make(map[string]*TSType, len(m.namedTypes))
	for qualName, ts := range m.namedTypes {
		tsName := m.tsNames[qualName]
		result[tsName] = &TSType{Name: tsName, Definition: ts.Definition}
	}
	return result
}

// qualifiedName returns the qualified name for a named type.
func qualifiedName(obj *types.TypeName) string {
	if obj.Pkg() != nil {
		return obj.Pkg().Path() + "." + obj.Name()
	}
	return obj.Name()
}

// registerName registers a named type and handles collisions.
// Returns the TypeScript name to use for references.
// On collision, the first registered type keeps its short name and subsequent ones get disambiguated.
func (m *TypeMapper) registerName(obj *types.TypeName) string {
	qualName := qualifiedName(obj)
	shortName := obj.Name()

	// Already registered
	if tsName, exists := m.tsNames[qualName]; exists {
		return tsName
	}

	// Check for short name collision
	if _, taken := m.shortNameOwners[shortName]; taken {
		// Collision: disambiguate the new type (first registered keeps short name)
		newName := disambiguate(qualName)
		m.tsNames[qualName] = newName
		return newName
	}

	// No collision
	m.shortNameOwners[shortName] = qualName
	m.tsNames[qualName] = shortName
	return shortName
}

// disambiguate creates a TypeScript name from a qualified Go name.
// e.g., "example.com/pkg/models.User" -> "ModelsUser"
func disambiguate(qualName string) string {
	parts := strings.Split(qualName, ".")
	if len(parts) < 2 {
		return qualName
	}
	typeName := parts[len(parts)-1]
	pkgPath := strings.Join(parts[:len(parts)-1], ".")

	// Extract the last segment of the package path
	pkgParts := strings.Split(pkgPath, "/")
	pkgShort := pkgParts[len(pkgParts)-1]

	return pascalCase(pkgShort) + typeName
}

// pascalCase converts a string to PascalCase.
func pascalCase(s string) string {
	if len(s) == 0 {
		return s
	}
	runes := []rune(s)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

// mapInstantiatedGeneric handles a named type with type arguments (e.g., Response[User]).
// It generates a unique name like "ResponseUser" and maps the resolved underlying type.
func (m *TypeMapper) mapInstantiatedGeneric(typ *types.Named) string {
	obj := typ.Obj()
	typeArgs := typ.TypeArgs()

	// Build a unique name: Response + User + ... → ResponseUser
	// Capitalize each arg name to produce valid PascalCase identifiers
	var argNames []string
	for i := 0; i < typeArgs.Len(); i++ {
		argTS := m.mapType(typeArgs.At(i), false)
		argNames = append(argNames, pascalCase(sanitizeIdentifier(argTS)))
	}
	tsName := obj.Name() + strings.Join(argNames, "")

	// Build a unique qualified key
	qualName := qualifiedName(obj) + "[" + strings.Join(argNames, ",") + "]"

	if _, exists := m.namedTypes[qualName]; exists {
		return m.tsNames[qualName]
	}

	// Check for collision with existing short names
	if existingQual, taken := m.shortNameOwners[tsName]; taken && existingQual != qualName {
		// Collision — disambiguate with package name
		tsName = disambiguate(qualifiedName(obj)) + strings.Join(argNames, "")
	}

	m.tsNames[qualName] = tsName
	m.shortNameOwners[tsName] = qualName

	underlying := typ.Underlying()
	if s, ok := underlying.(*types.Struct); ok {
		m.namedTypes[qualName] = &TSType{Name: tsName}
		def := m.mapStructType(s)
		m.namedTypes[qualName].Definition = def
	} else {
		underlyingTS := m.mapType(underlying, false)
		m.namedTypes[qualName] = &TSType{Name: tsName, Definition: underlyingTS}
	}

	return tsName
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

		// Check for well-known types
		if obj.Pkg() != nil {
			fullName := obj.Pkg().Path() + "." + obj.Name()
			if tsType, ok := wellKnownTypes[fullName]; ok {
				return tsType
			}
		}

		// Handle instantiated generic types (e.g., Response[User])
		if typeArgs := typ.TypeArgs(); typeArgs != nil && typeArgs.Len() > 0 {
			return m.mapInstantiatedGeneric(typ)
		}

		qualName := qualifiedName(obj)

		// If it's a struct, register and return by name
		underlying := typ.Underlying()
		if _, ok := underlying.(*types.Struct); ok {
			tsName := m.registerName(obj)
			if _, exists := m.namedTypes[qualName]; !exists {
				// Placeholder to prevent infinite recursion
				m.namedTypes[qualName] = &TSType{Name: tsName}
				def := m.mapStructType(underlying.(*types.Struct))
				m.namedTypes[qualName].Definition = def
			}
			return m.tsNames[qualName]
		}

		// Check if this is an enum type (lookup by qualified name)
		if enumInfo, ok := m.enumTypes[qualName]; ok {
			tsName := m.registerName(obj)
			if _, exists := m.namedTypes[qualName]; !exists {
				var def string
				if enumInfo.IsString {
					quoted := make([]string, len(enumInfo.Values))
					for i, v := range enumInfo.Values {
						quoted[i] = fmt.Sprintf("%q", v)
					}
					def = strings.Join(quoted, " | ")
				} else {
					def = strings.Join(enumInfo.Values, " | ")
				}
				m.namedTypes[qualName] = &TSType{Name: tsName, Definition: def}
			}
			return m.tsNames[qualName]
		}

		// For non-struct named types (e.g., type UserID string), register as a named alias
		tsName := m.registerName(obj)
		if _, exists := m.namedTypes[qualName]; !exists {
			underlyingTS := m.mapType(underlying, false)
			m.namedTypes[qualName] = &TSType{Name: tsName, Definition: underlyingTS}
		}
		return m.tsNames[qualName]

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
		return wrapArray(elem)

	case *types.Array:
		elem := m.mapType(typ.Elem(), false)
		return wrapArray(elem)

	case *types.Map:
		val := m.mapType(typ.Elem(), false)
		return fmt.Sprintf("Record<string, %s>", val)

	case *types.Struct:
		return m.mapStructType(typ)

	case *types.Interface:
		return "unknown"

	case *types.TypeParam:
		// Uninstantiated type parameter — map to its constraint or unknown
		constraint := typ.Constraint()
		if constraint != nil {
			// If constraint is 'any' (empty interface), just use unknown
			if iface, ok := constraint.Underlying().(*types.Interface); ok && iface.NumMethods() == 0 && iface.NumEmbeddeds() == 0 {
				return "unknown"
			}
		}
		return "unknown"

	default:
		return "unknown"
	}
}

func (m *TypeMapper) mapStructType(s *types.Struct) string {
	if s.NumFields() == 0 {
		return "Record<string, never>"
	}

	var embeddedTypes []string
	var fields []string

	for i := 0; i < s.NumFields(); i++ {
		field := s.Field(i)
		if !field.Exported() {
			continue
		}

		// Handle embedded (anonymous) fields
		if field.Embedded() {
			// Get the embedded type, unwrapping pointers
			embType := field.Type()
			if ptr, ok := embType.(*types.Pointer); ok {
				embType = ptr.Elem()
			}

			// If it's a named struct, use intersection type
			if named, ok := embType.(*types.Named); ok {
				if _, ok := named.Underlying().(*types.Struct); ok {
					tsRef := m.mapType(embType, false)
					embeddedTypes = append(embeddedTypes, tsRef)
					continue
				}
			}

			// For non-named embedded types, flatten fields (fallback)
			if embStruct, ok := embType.Underlying().(*types.Struct); ok {
				for j := 0; j < embStruct.NumFields(); j++ {
					embField := embStruct.Field(j)
					if !embField.Exported() {
						continue
					}
					tag := embStruct.Tag(j)
					jsonName, opts := parseJSONTag(tag)
					if jsonName == "-" {
						continue
					}
					name := jsonName
					if name == "" {
						name = lowerFirst(embField.Name())
					}
					tsType := m.mapType(embField.Type(), false)
					if opts.stringTag {
						tsType = "string"
					}
					optional := ""
					if opts.omitempty {
						optional = "?"
					}
					fields = append(fields, fmt.Sprintf("  %s%s: %s;", name, optional, tsType))
				}
			}
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
		if opts.stringTag {
			tsType = "string"
		}
		optional := ""
		if opts.omitempty {
			optional = "?"
		}

		fields = append(fields, fmt.Sprintf("  %s%s: %s;", name, optional, tsType))
	}

	ownFields := ""
	if len(fields) > 0 {
		ownFields = "{\n" + strings.Join(fields, "\n") + "\n}"
	}

	if len(embeddedTypes) == 0 {
		if ownFields == "" {
			return "Record<string, never>"
		}
		return ownFields
	}

	// Build intersection type: EmbeddedA & EmbeddedB & { ownFields }
	parts := append(embeddedTypes, ownFields)
	// Filter empty parts
	var nonEmpty []string
	for _, p := range parts {
		if p != "" {
			nonEmpty = append(nonEmpty, p)
		}
	}

	if len(nonEmpty) == 1 {
		return nonEmpty[0]
	}
	return strings.Join(nonEmpty, " & ")
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
	stringTag bool // json:",string" — marshal as JSON string on the wire
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
		switch p {
		case "omitempty":
			opts.omitempty = true
		case "string":
			opts.stringTag = true
		}
	}
	return name, opts
}

// wrapArray adds [] to a type, wrapping in parentheses if the type contains | (union).
// e.g., "User | null" → "(User | null)[]", "string" → "string[]"
func wrapArray(elem string) string {
	if strings.Contains(elem, " | ") {
		return "(" + elem + ")[]"
	}
	return elem + "[]"
}

// sanitizeIdentifier strips all non-letter/digit characters from a string,
// producing a valid identifier fragment. Used for generic type arg names
// where the TS type may contain brackets, angles, etc. (e.g., "User[]" → "User").
func sanitizeIdentifier(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func lowerFirst(s string) string {
	if len(s) == 0 {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}
