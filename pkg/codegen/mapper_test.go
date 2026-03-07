package codegen

import (
	"go/types"
	"strings"
	"testing"
	"unicode"
)

// ---------------------------------------------------------------------------
// Test helpers — reduce boilerplate for constructing go/types objects
// ---------------------------------------------------------------------------

// testPkg creates a types.Package for use in tests.
func testPkg(path, name string) *types.Package {
	return types.NewPackage(path, name)
}

// makeNamedType creates a *types.Named with the given package, name, and underlying type.
// Simulates declarations like: type <name> <underlying>
func makeNamedType(pkg *types.Package, name string, underlying types.Type) *types.Named {
	tn := types.NewTypeName(0, pkg, name, nil)
	return types.NewNamed(tn, underlying, nil)
}

// makeStruct creates a *types.Struct from field definitions.
// Each field is (name, type, embedded, jsonTag).
func makeStruct(pkg *types.Package, fields []structField) *types.Struct {
	vars := make([]*types.Var, len(fields))
	tags := make([]string, len(fields))
	for i, f := range fields {
		vars[i] = types.NewField(0, pkg, f.Name, f.Type, f.Embedded)
		tags[i] = f.Tag
	}
	return types.NewStruct(vars, tags)
}

type structField struct {
	Name     string
	Type     types.Type
	Embedded bool
	Tag      string
}

// makeGenericType creates a generic named type with one type param T and a struct
// body built from the provided fields. Use the returned *types.TypeParam as a
// field type to reference T in the struct body.
func makeGenericType(pkg *types.Package, name string, fields []structField) (*types.Named, *types.TypeParam) {
	tpName := types.NewTypeName(0, pkg, "T", nil)
	tp := types.NewTypeParam(tpName, types.NewInterfaceType(nil, nil))

	tn := types.NewTypeName(0, pkg, name, nil)
	named := types.NewNamed(tn, nil, nil)
	named.SetTypeParams([]*types.TypeParam{tp})

	s := makeStruct(pkg, fields)
	named.SetUnderlying(s)

	return named, tp
}

// ---------------------------------------------------------------------------
// 1. Basic type mapping — primitive Go types → TypeScript types
// ---------------------------------------------------------------------------

func TestBasicTypes(t *testing.T) {
	t.Run("primitives", func(t *testing.T) {
		mapper := NewTypeMapper()
		tests := []struct {
			name   string
			goType types.BasicKind
			want   string
		}{
			{"string", types.String, "string"},
			{"bool", types.Bool, "boolean"},
			{"int", types.Int, "number"},
			{"int8", types.Int8, "number"},
			{"int16", types.Int16, "number"},
			{"int32", types.Int32, "number"},
			{"int64", types.Int64, "number"},
			{"uint", types.Uint, "number"},
			{"uint8", types.Uint8, "number"},
			{"uint16", types.Uint16, "number"},
			{"uint32", types.Uint32, "number"},
			{"uint64", types.Uint64, "number"},
			{"float32", types.Float32, "number"},
			{"float64", types.Float64, "number"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got := mapper.MapType(types.Typ[tt.goType])
				if got != tt.want {
					t.Errorf("MapType(%s) = %q, want %q", tt.name, got, tt.want)
				}
			})
		}
	})

	t.Run("pointer produces union with null", func(t *testing.T) {
		mapper := NewTypeMapper()
		got := mapper.MapType(types.NewPointer(types.Typ[types.String]))
		assertEq(t, got, "string | null")
	})

	t.Run("interface{} maps to unknown", func(t *testing.T) {
		mapper := NewTypeMapper()
		got := mapper.MapType(types.NewInterfaceType(nil, nil))
		assertEq(t, got, "unknown")
	})
}

// ---------------------------------------------------------------------------
// 2. Collection types — slices, arrays, maps, []byte
// ---------------------------------------------------------------------------

func TestCollectionTypes(t *testing.T) {
	t.Run("[]string → string[]", func(t *testing.T) {
		mapper := NewTypeMapper()
		got := mapper.MapType(types.NewSlice(types.Typ[types.String]))
		assertEq(t, got, "string[]")
	})

	t.Run("[]byte → string (base64 convention)", func(t *testing.T) {
		mapper := NewTypeMapper()
		got := mapper.MapType(types.NewSlice(types.Typ[types.Byte]))
		assertEq(t, got, "string")
	})

	t.Run("[]*string → (string | null)[] with parentheses", func(t *testing.T) {
		// Verifies that union types inside arrays are wrapped in parens.
		// Without this fix: "string | null[]" which TypeScript parses as "string | (null[])"
		mapper := NewTypeMapper()
		got := mapper.MapType(types.NewSlice(types.NewPointer(types.Typ[types.String])))
		assertEq(t, got, "(string | null)[]")
	})

	t.Run("[4]int → number[]", func(t *testing.T) {
		mapper := NewTypeMapper()
		got := mapper.MapType(types.NewArray(types.Typ[types.Int], 4))
		assertEq(t, got, "number[]")
	})

	t.Run("[]*int → (number | null)[] array of pointers", func(t *testing.T) {
		mapper := NewTypeMapper()
		got := mapper.MapType(types.NewArray(types.NewPointer(types.Typ[types.Int]), 4))
		assertEq(t, got, "(number | null)[]")
	})

	t.Run("map[string]int → Record<string, number>", func(t *testing.T) {
		mapper := NewTypeMapper()
		got := mapper.MapType(types.NewMap(types.Typ[types.String], types.Typ[types.Int]))
		assertEq(t, got, "Record<string, number>")
	})
}

// ---------------------------------------------------------------------------
// 3. Well-known types — external types with special TypeScript mappings
// ---------------------------------------------------------------------------

func TestWellKnownTypes(t *testing.T) {
	tests := []struct {
		name       string
		pkgPath    string
		pkgName    string
		typeName   string
		underlying types.Type
		want       string
	}{
		{"time.Time → string", "time", "time", "Time", types.Typ[types.Int64], "string"},
		{"uuid.UUID → string", "github.com/google/uuid", "uuid", "UUID", types.NewArray(types.Typ[types.Byte], 16), "string"},
		{"sql.NullString → string | null", "database/sql", "sql", "NullString", types.Typ[types.String], "string | null"},
		{"sql.NullInt64 → number | null", "database/sql", "sql", "NullInt64", types.Typ[types.Int64], "number | null"},
		{"sql.NullBool → boolean | null", "database/sql", "sql", "NullBool", types.Typ[types.Bool], "boolean | null"},
		{"json.RawMessage → unknown", "encoding/json", "json", "RawMessage", types.NewSlice(types.Typ[types.Byte]), "unknown"},
		{"decimal.Decimal → string", "github.com/shopspring/decimal", "decimal", "Decimal", types.Typ[types.String], "string"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mapper := NewTypeMapper()
			named := makeNamedType(testPkg(tt.pkgPath, tt.pkgName), tt.typeName, tt.underlying)
			got := mapper.MapType(named)
			assertEq(t, got, tt.want)
		})
	}
}

// ---------------------------------------------------------------------------
// 4. Named type aliases — non-struct named types preserve their name
// ---------------------------------------------------------------------------

func TestNamedTypeAliases(t *testing.T) {
	t.Run("type UserID string → type UserID = string", func(t *testing.T) {
		// Named types with non-struct underlying types should be emitted as
		// TypeScript type aliases, preserving the semantic name.
		mapper := NewTypeMapper()
		pkg := testPkg("example.com/models", "models")
		named := makeNamedType(pkg, "UserID", types.Typ[types.String])

		got := mapper.MapType(named)
		assertEq(t, got, "UserID")

		nt := mapper.NamedTypes()
		ts, ok := nt["UserID"]
		if !ok {
			t.Fatal("UserID not registered in named types")
		}
		assertEq(t, ts.Definition, "string")
	})

	t.Run("type Count int → type Count = number", func(t *testing.T) {
		mapper := NewTypeMapper()
		pkg := testPkg("example.com/models", "models")
		named := makeNamedType(pkg, "Count", types.Typ[types.Int])

		got := mapper.MapType(named)
		assertEq(t, got, "Count")

		nt := mapper.NamedTypes()
		ts, ok := nt["Count"]
		if !ok {
			t.Fatal("Count not registered in named types")
		}
		assertEq(t, ts.Definition, "number")
	})
}

// ---------------------------------------------------------------------------
// 5. Enums — Go idiomatic enum pattern (type X string + const block)
// ---------------------------------------------------------------------------

func TestEnums(t *testing.T) {
	t.Run("string enum → union of string literals", func(t *testing.T) {
		// Go pattern: type Status string; const ( Active Status = "active"; ... )
		// Expected TS: type Status = "active" | "inactive" | "pending"
		mapper := NewTypeMapper()
		mapper.RegisterEnums([]EnumInfo{
			{TypeName: "Status", QualifiedName: "example.com/models.Status", Values: []string{"active", "inactive", "pending"}, IsString: true},
		})

		pkg := testPkg("example.com/models", "models")
		named := makeNamedType(pkg, "Status", types.Typ[types.String])

		got := mapper.MapType(named)
		assertEq(t, got, "Status")

		nt := mapper.NamedTypes()
		ts := nt["Status"]
		assertEq(t, ts.Definition, `"active" | "inactive" | "pending"`)
	})

	t.Run("int enum → union of number literals", func(t *testing.T) {
		// Go pattern: type Priority int; const ( Low Priority = 0; ... )
		// Expected TS: type Priority = 0 | 1 | 2
		mapper := NewTypeMapper()
		mapper.RegisterEnums([]EnumInfo{
			{TypeName: "Priority", QualifiedName: "example.com/models.Priority", Values: []string{"0", "1", "2"}, IsString: false},
		})

		pkg := testPkg("example.com/models", "models")
		named := makeNamedType(pkg, "Priority", types.Typ[types.Int])

		got := mapper.MapType(named)
		assertEq(t, got, "Priority")

		nt := mapper.NamedTypes()
		ts := nt["Priority"]
		assertEq(t, ts.Definition, "0 | 1 | 2")
	})

	t.Run("enum lookup uses qualified name to avoid cross-package false matches", func(t *testing.T) {
		// If models.Status and auth.Status both exist, the enum info for
		// models.Status should NOT match auth.Status.
		mapper := NewTypeMapper()
		mapper.RegisterEnums([]EnumInfo{
			{TypeName: "Status", QualifiedName: "example.com/models.Status", Values: []string{"active"}, IsString: true},
		})

		// auth.Status is NOT an enum — should fall through to named alias
		authPkg := testPkg("example.com/auth", "auth")
		authStatus := makeNamedType(authPkg, "Status", types.Typ[types.String])

		got := mapper.MapType(authStatus)
		assertEq(t, got, "Status") // gets short name (no collision yet)

		nt := mapper.NamedTypes()
		ts := nt["Status"]
		// Should be "string" (plain alias), NOT an enum union
		assertEq(t, ts.Definition, "string")
	})
}

// ---------------------------------------------------------------------------
// 6. Generics — type parameters and instantiated generic types
// ---------------------------------------------------------------------------

func TestGenerics(t *testing.T) {
	t.Run("uninstantiated type param T any → unknown", func(t *testing.T) {
		mapper := NewTypeMapper()
		tp := types.NewTypeParam(types.NewTypeName(0, nil, "T", nil), types.NewInterfaceType(nil, nil))
		got := mapper.MapType(tp)
		assertEq(t, got, "unknown")
	})

	t.Run("Response[User] → ResponseUser with resolved fields", func(t *testing.T) {
		mapper := NewTypeMapper()
		pkg := testPkg("example.com/models", "models")

		tpName := types.NewTypeName(0, pkg, "T", nil)
		tp := types.NewTypeParam(tpName, types.NewInterfaceType(nil, nil))

		generic, _ := makeGenericType(pkg, "Response", []structField{
			{Name: "Data", Type: tp, Tag: `json:"data"`},
		})

		userNamed := makeNamedType(pkg, "User", makeStruct(pkg, []structField{
			{Name: "Name", Type: types.Typ[types.String], Tag: `json:"name"`},
		}))

		instantiated, err := types.Instantiate(nil, generic, []types.Type{userNamed}, false)
		if err != nil {
			t.Fatalf("Instantiate failed: %v", err)
		}

		got := mapper.MapType(instantiated)
		assertEq(t, got, "ResponseUser")

		nt := mapper.NamedTypes()
		if _, ok := nt["ResponseUser"]; !ok {
			t.Error("ResponseUser not found in named types")
		}
	})

	t.Run("Response[string] → ResponseString with PascalCase", func(t *testing.T) {
		// Primitive type args must be capitalized: "Responsestring" is not valid PascalCase.
		mapper := NewTypeMapper()
		pkg := testPkg("example.com/models", "models")

		tpName := types.NewTypeName(0, pkg, "T", nil)
		tp := types.NewTypeParam(tpName, types.NewInterfaceType(nil, nil))

		generic, _ := makeGenericType(pkg, "Response", []structField{
			{Name: "Data", Type: tp, Tag: `json:"data"`},
		})

		instantiated, err := types.Instantiate(nil, generic, []types.Type{types.Typ[types.String]}, false)
		if err != nil {
			t.Fatalf("Instantiate failed: %v", err)
		}

		got := mapper.MapType(instantiated)
		assertEq(t, got, "ResponseString")
	})

	t.Run("Response[[]User] → sanitized valid identifier", func(t *testing.T) {
		// Complex type args like []User map to "User[]" which contains brackets.
		// sanitizeIdentifier strips non-alphanumeric chars → "User" → "ResponseUser".
		mapper := NewTypeMapper()
		pkg := testPkg("example.com/models", "models")

		userNamed := makeNamedType(pkg, "User", makeStruct(pkg, []structField{
			{Name: "Name", Type: types.Typ[types.String], Tag: `json:"name"`},
		}))

		generic, _ := makeGenericType(pkg, "Response", []structField{
			{Name: "Data", Type: types.NewTypeParam(
				types.NewTypeName(0, pkg, "T", nil),
				types.NewInterfaceType(nil, nil),
			), Tag: `json:"data"`},
		})

		// Instantiate with []User (a slice type)
		sliceOfUser := types.NewSlice(userNamed)
		instantiated, err := types.Instantiate(nil, generic, []types.Type{sliceOfUser}, false)
		if err != nil {
			t.Fatalf("Instantiate failed: %v", err)
		}

		got := mapper.MapType(instantiated)
		// Name should not contain [] or other invalid identifier chars
		for _, r := range got {
			if !isIdentChar(r) {
				t.Errorf("generated name %q contains invalid char %q", got, string(r))
			}
		}
		// Should be something like "ResponseUser" (sanitized from "User[]")
		if got == "" {
			t.Error("generated name should not be empty")
		}
	})

	t.Run("Response[map[string]int] → sanitized valid identifier", func(t *testing.T) {
		mapper := NewTypeMapper()
		pkg := testPkg("example.com/models", "models")

		generic, _ := makeGenericType(pkg, "Response", []structField{
			{Name: "Data", Type: types.NewTypeParam(
				types.NewTypeName(0, pkg, "T", nil),
				types.NewInterfaceType(nil, nil),
			), Tag: `json:"data"`},
		})

		mapType := types.NewMap(types.Typ[types.String], types.Typ[types.Int])
		instantiated, err := types.Instantiate(nil, generic, []types.Type{mapType}, false)
		if err != nil {
			t.Fatalf("Instantiate failed: %v", err)
		}

		got := mapper.MapType(instantiated)
		for _, r := range got {
			if !isIdentChar(r) {
				t.Errorf("generated name %q contains invalid char %q", got, string(r))
			}
		}
	})
}

// ---------------------------------------------------------------------------
// 7. Embedded structs — intersection types via &
// ---------------------------------------------------------------------------

func TestEmbeddedStructs(t *testing.T) {
	pkg := testPkg("example.com/models", "models")

	// Shared base struct: type Base struct { ID int `json:"id"` }
	baseNamed := makeNamedType(pkg, "Base", makeStruct(pkg, []structField{
		{Name: "ID", Type: types.Typ[types.Int], Tag: `json:"id"`},
	}))

	t.Run("embedded named struct → intersection type with &", func(t *testing.T) {
		// Go: type Child struct { Base; Name string }
		// TS:  Base & { name: string; }
		mapper := NewTypeMapper()
		childStruct := makeStruct(pkg, []structField{
			{Name: "Base", Type: baseNamed, Embedded: true},
			{Name: "Name", Type: types.Typ[types.String], Tag: `json:"name"`},
		})

		got := mapper.mapStructType(childStruct)

		assertContains(t, got, "Base", "expected embedded type reference")
		assertContains(t, got, "&", "expected intersection operator")
		assertContains(t, got, "name: string", "expected own field")
	})

	t.Run("only embedded struct → returns just the base type reference", func(t *testing.T) {
		// Go: type Wrapper struct { Base }
		// TS:  Base (no & needed when there are no own fields)
		mapper := NewTypeMapper()
		wrapperStruct := makeStruct(pkg, []structField{
			{Name: "Base", Type: baseNamed, Embedded: true},
		})

		got := mapper.mapStructType(wrapperStruct)
		assertEq(t, got, "Base")
	})
}

// ---------------------------------------------------------------------------
// 8. Name collision disambiguation — same short name from different packages
// ---------------------------------------------------------------------------

func TestNameCollisions(t *testing.T) {
	t.Run("two User types from different packages → first keeps short name", func(t *testing.T) {
		// models.User registered first → "User"
		// auth.User registered second → "AuthUser"
		mapper := NewTypeMapper()

		modelsPkg := testPkg("example.com/models", "models")
		authPkg := testPkg("example.com/auth", "auth")

		modelsUser := makeNamedType(modelsPkg, "User", makeStruct(modelsPkg, []structField{
			{Name: "Name", Type: types.Typ[types.String], Tag: `json:"name"`},
		}))
		authUser := makeNamedType(authPkg, "User", makeStruct(authPkg, []structField{
			{Name: "Email", Type: types.Typ[types.String], Tag: `json:"email"`},
		}))

		ref1 := mapper.MapType(modelsUser)
		ref2 := mapper.MapType(authUser)

		assertEq(t, ref1, "User")
		assertEq(t, ref2, "AuthUser")

		nt := mapper.NamedTypes()
		if len(nt) != 2 {
			t.Errorf("expected 2 named types, got %d", len(nt))
		}
		if _, ok := nt["User"]; !ok {
			t.Error("User not found in named types")
		}
		if _, ok := nt["AuthUser"]; !ok {
			t.Error("AuthUser not found in named types")
		}
	})
}

// ---------------------------------------------------------------------------
// 9. Recursive types — self-referential structs
// ---------------------------------------------------------------------------

func TestRecursiveTypes(t *testing.T) {
	pkg := testPkg("example.com/models", "models")

	t.Run("type Tree struct { Children []*Tree } — self-reference via pointer+slice", func(t *testing.T) {
		mapper := NewTypeMapper()

		treeTN := types.NewTypeName(0, pkg, "Tree", nil)
		treeNamed := types.NewNamed(treeTN, nil, nil)
		treeStruct := makeStruct(pkg, []structField{
			{Name: "Name", Type: types.Typ[types.String], Tag: `json:"name"`},
			{Name: "Children", Type: types.NewSlice(types.NewPointer(treeNamed)), Tag: `json:"children"`},
		})
		treeNamed.SetUnderlying(treeStruct)

		got := mapper.MapType(treeNamed)
		assertEq(t, got, "Tree")

		ts := mapper.NamedTypes()["Tree"]
		assertContains(t, ts.Definition, "name: string", "expected name field")
		assertContains(t, ts.Definition, "(Tree | null)[]", "expected recursive self-reference with parens")
	})

	t.Run("recursive type with embedded struct", func(t *testing.T) {
		// type Node struct { Base; Children []*Node }
		mapper := NewTypeMapper()

		baseNamed := makeNamedType(pkg, "Base", makeStruct(pkg, []structField{
			{Name: "ID", Type: types.Typ[types.Int], Tag: `json:"id"`},
		}))

		nodeTN := types.NewTypeName(0, pkg, "Node", nil)
		nodeNamed := types.NewNamed(nodeTN, nil, nil)
		nodeStruct := makeStruct(pkg, []structField{
			{Name: "Base", Type: baseNamed, Embedded: true},
			{Name: "Children", Type: types.NewSlice(types.NewPointer(nodeNamed)), Tag: `json:"children"`},
		})
		nodeNamed.SetUnderlying(nodeStruct)

		got := mapper.MapType(nodeNamed)
		assertEq(t, got, "Node")

		ts := mapper.NamedTypes()["Node"]
		assertContains(t, ts.Definition, "Base", "expected intersection with embedded Base")
		assertContains(t, ts.Definition, "(Node | null)[]", "expected recursive self-reference")
	})
}

// ---------------------------------------------------------------------------
// 10. json:",string" tag — wire type override
// ---------------------------------------------------------------------------

func TestStringTagOverride(t *testing.T) {
	pkg := testPkg("example.com/models", "models")

	t.Run("int field with json string tag → string in TS", func(t *testing.T) {
		// Go: Count int `json:"count,string"` → marshaled as "123" on the wire
		// TS:  count: string
		mapper := NewTypeMapper()
		s := makeStruct(pkg, []structField{
			{Name: "Count", Type: types.Typ[types.Int], Tag: `json:"count,string"`},
		})
		got := mapper.mapStructType(s)
		assertContains(t, got, "count: string", "int with ,string should map to TS string")
	})

	t.Run("bool field with json string tag → string in TS", func(t *testing.T) {
		mapper := NewTypeMapper()
		s := makeStruct(pkg, []structField{
			{Name: "Active", Type: types.Typ[types.Bool], Tag: `json:"active,string"`},
		})
		got := mapper.mapStructType(s)
		assertContains(t, got, "active: string", "bool with ,string should map to TS string")
	})

	t.Run("combined omitempty and string → optional string", func(t *testing.T) {
		mapper := NewTypeMapper()
		s := makeStruct(pkg, []structField{
			{Name: "Count", Type: types.Typ[types.Int], Tag: `json:"count,omitempty,string"`},
		})
		got := mapper.mapStructType(s)
		assertContains(t, got, "count?: string", "combined omitempty+string should produce optional string")
	})

	t.Run("string field with json string tag → stays string", func(t *testing.T) {
		mapper := NewTypeMapper()
		s := makeStruct(pkg, []structField{
			{Name: "Name", Type: types.Typ[types.String], Tag: `json:"name,string"`},
		})
		got := mapper.mapStructType(s)
		assertContains(t, got, "name: string", "string with ,string should stay string")
	})
}

// ---------------------------------------------------------------------------
// 11. Utility functions — lowerFirst, parseJSONTag, sanitizeIdentifier
// ---------------------------------------------------------------------------

func TestUtilities(t *testing.T) {
	t.Run("lowerFirst", func(t *testing.T) {
		tests := []struct {
			input, want string
		}{
			{"Name", "name"},
			{"ID", "iD"},
			{"", ""},
			{"a", "a"},
		}
		for _, tt := range tests {
			got := lowerFirst(tt.input)
			if got != tt.want {
				t.Errorf("lowerFirst(%q) = %q, want %q", tt.input, got, tt.want)
			}
		}
	})

	t.Run("parseJSONTag", func(t *testing.T) {
		tests := []struct {
			tag        string
			wantName   string
			wantOmit   bool
			wantString bool
		}{
			{`json:"name"`, "name", false, false},
			{`json:"name,omitempty"`, "name", true, false},
			{`json:"-"`, "-", false, false},
			{`json:"id" db:"id"`, "id", false, false},
			{``, "", false, false},
			{`json:"field_name,omitempty"`, "field_name", true, false},
			{`json:"count,string"`, "count", false, true},
			{`json:"count,omitempty,string"`, "count", true, true},
		}
		for _, tt := range tests {
			name, opts := parseJSONTag(tt.tag)
			if name != tt.wantName {
				t.Errorf("parseJSONTag(%q) name = %q, want %q", tt.tag, name, tt.wantName)
			}
			if opts.omitempty != tt.wantOmit {
				t.Errorf("parseJSONTag(%q) omitempty = %v, want %v", tt.tag, opts.omitempty, tt.wantOmit)
			}
			if opts.stringTag != tt.wantString {
				t.Errorf("parseJSONTag(%q) stringTag = %v, want %v", tt.tag, opts.stringTag, tt.wantString)
			}
		}
	})

	t.Run("sanitizeIdentifier", func(t *testing.T) {
		tests := []struct {
			input, want string
		}{
			{"User", "User"},
			{"User[]", "User"},
			{"(User | null)[]", "Usernull"},
			{"Record<string, number>", "Recordstringnumber"},
			{"string", "string"},
			{"", ""},
		}
		for _, tt := range tests {
			got := sanitizeIdentifier(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeIdentifier(%q) = %q, want %q", tt.input, got, tt.want)
			}
		}
	})
}

// ---------------------------------------------------------------------------
// Test assertion helpers
// ---------------------------------------------------------------------------

func assertEq(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func assertContains(t *testing.T, s, substr, msg string) {
	t.Helper()
	if !strings.Contains(s, substr) {
		t.Errorf("%s: %q does not contain %q", msg, s, substr)
	}
}

// isIdentChar returns true if the rune is valid in a TypeScript/JS identifier.
func isIdentChar(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '$'
}
