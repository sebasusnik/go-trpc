package codegen

import (
	"go/types"
	"testing"
)

func TestMapBasicTypes(t *testing.T) {
	mapper := NewTypeMapper()

	tests := []struct {
		goType types.BasicKind
		want   string
	}{
		{types.String, "string"},
		{types.Bool, "boolean"},
		{types.Int, "number"},
		{types.Int8, "number"},
		{types.Int16, "number"},
		{types.Int32, "number"},
		{types.Int64, "number"},
		{types.Uint, "number"},
		{types.Uint8, "number"},
		{types.Uint16, "number"},
		{types.Uint32, "number"},
		{types.Uint64, "number"},
		{types.Float32, "number"},
		{types.Float64, "number"},
	}

	for _, tt := range tests {
		got := mapper.MapType(types.Typ[tt.goType])
		if got != tt.want {
			t.Errorf("MapType(%v) = %q, want %q", tt.goType, got, tt.want)
		}
	}
}

func TestMapPointerType(t *testing.T) {
	mapper := NewTypeMapper()

	ptrType := types.NewPointer(types.Typ[types.String])
	got := mapper.MapType(ptrType)
	if got != "string | null" {
		t.Errorf("MapType(*string) = %q, want %q", got, "string | null")
	}
}

func TestMapSliceType(t *testing.T) {
	mapper := NewTypeMapper()

	sliceType := types.NewSlice(types.Typ[types.String])
	got := mapper.MapType(sliceType)
	if got != "string[]" {
		t.Errorf("MapType([]string) = %q, want %q", got, "string[]")
	}
}

func TestMapByteSlice(t *testing.T) {
	mapper := NewTypeMapper()

	byteSlice := types.NewSlice(types.Typ[types.Byte])
	got := mapper.MapType(byteSlice)
	if got != "string" {
		t.Errorf("MapType([]byte) = %q, want %q", got, "string")
	}
}

func TestMapMapType(t *testing.T) {
	mapper := NewTypeMapper()

	mapType := types.NewMap(types.Typ[types.String], types.Typ[types.Int])
	got := mapper.MapType(mapType)
	if got != "Record<string, number>" {
		t.Errorf("MapType(map[string]int) = %q, want %q", got, "Record<string, number>")
	}
}

func TestMapInterfaceType(t *testing.T) {
	mapper := NewTypeMapper()

	iface := types.NewInterfaceType(nil, nil)
	got := mapper.MapType(iface)
	if got != "unknown" {
		t.Errorf("MapType(interface{}) = %q, want %q", got, "unknown")
	}
}

func TestParseJSONTag(t *testing.T) {
	tests := []struct {
		tag       string
		wantName  string
		wantOmit  bool
	}{
		{`json:"name"`, "name", false},
		{`json:"name,omitempty"`, "name", true},
		{`json:"-"`, "-", false},
		{`json:"id" db:"id"`, "id", false},
		{``, "", false},
		{`json:"field_name,omitempty"`, "field_name", true},
	}

	for _, tt := range tests {
		name, opts := parseJSONTag(tt.tag)
		if name != tt.wantName {
			t.Errorf("parseJSONTag(%q) name = %q, want %q", tt.tag, name, tt.wantName)
		}
		if opts.omitempty != tt.wantOmit {
			t.Errorf("parseJSONTag(%q) omitempty = %v, want %v", tt.tag, opts.omitempty, tt.wantOmit)
		}
	}
}

func TestLowerFirst(t *testing.T) {
	tests := []struct {
		input string
		want  string
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
}
