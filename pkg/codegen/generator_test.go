package codegen

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateFromProcedures_SingleQuery(t *testing.T) {
	procs := []ProcedureInfo{
		{Name: "getUser", Type: "query"},
	}

	output := GenerateFromProcedures(procs, "AppRouter")

	if !strings.Contains(output, "DO NOT EDIT") {
		t.Error("expected generated header comment")
	}
	if !strings.Contains(output, "getUser:") {
		t.Error("expected procedure name 'getUser'")
	}
	if !strings.Contains(output, `type: "query"`) {
		t.Error("expected query type")
	}
	if !strings.Contains(output, "export type { AppRouter }") {
		t.Error("expected AppRouter export")
	}
	if !strings.Contains(output, `"getUser": void`) {
		t.Error("expected RouterInputs entry for getUser")
	}
}

func TestGenerateFromProcedures_Mutation(t *testing.T) {
	procs := []ProcedureInfo{
		{Name: "createUser", Type: "mutation"},
	}

	output := GenerateFromProcedures(procs, "AppRouter")

	if !strings.Contains(output, `type: "mutation"`) {
		t.Error("expected mutation type")
	}
}

func TestGenerateFromProcedures_NestedRouter(t *testing.T) {
	procs := []ProcedureInfo{
		{Name: "user.get", Type: "query"},
		{Name: "user.create", Type: "mutation"},
	}

	output := GenerateFromProcedures(procs, "AppRouter")

	// Should have a 'user' namespace containing 'get' and 'create'
	if !strings.Contains(output, "user: {") {
		t.Error("expected 'user' namespace")
	}
	if !strings.Contains(output, "get:") {
		t.Error("expected 'get' procedure inside user namespace")
	}
	if !strings.Contains(output, "create:") {
		t.Error("expected 'create' procedure inside user namespace")
	}
}

func TestGenerateFromProcedures_EmptyProducesHeader(t *testing.T) {
	output := GenerateFromProcedures(nil, "AppRouter")

	if !strings.Contains(output, "DO NOT EDIT") {
		t.Error("expected header comment even with no procedures")
	}
	if !strings.Contains(output, "AppRouter") {
		t.Error("expected AppRouter type definition")
	}
}

func TestGenerateFromProcedures_CustomRouterName(t *testing.T) {
	procs := []ProcedureInfo{
		{Name: "hello", Type: "query"},
	}

	output := GenerateFromProcedures(procs, "MyRouter")

	if !strings.Contains(output, "interface MyRouter") {
		t.Error("expected custom router name")
	}
	if !strings.Contains(output, "export type { MyRouter }") {
		t.Error("expected custom router name in export")
	}
}

func TestResolvePrefixes_SingleLevel(t *testing.T) {
	result := &ParseResult{
		Procedures: []ProcedureInfo{
			{Name: "get", RouterVar: "taskRouter"},
		},
		Merges: []MergeInfo{
			{ParentVar: "r", Prefix: "task", ChildVar: "taskRouter"},
		},
		RouterVar: "r",
	}

	resolvePrefixes(result)

	if result.Procedures[0].Name != "task.get" {
		t.Errorf("expected 'task.get', got %q", result.Procedures[0].Name)
	}
}

func TestResolvePrefixes_Nested(t *testing.T) {
	result := &ParseResult{
		Procedures: []ProcedureInfo{
			{Name: "list", RouterVar: "itemRouter"},
		},
		Merges: []MergeInfo{
			{ParentVar: "r", Prefix: "api", ChildVar: "apiRouter"},
			{ParentVar: "apiRouter", Prefix: "items", ChildVar: "itemRouter"},
		},
		RouterVar: "r",
	}

	resolvePrefixes(result)

	if result.Procedures[0].Name != "api.items.list" {
		t.Errorf("expected 'api.items.list', got %q", result.Procedures[0].Name)
	}
}

func TestResolvePrefixes_NoMerges(t *testing.T) {
	result := &ParseResult{
		Procedures: []ProcedureInfo{
			{Name: "hello", RouterVar: "r"},
		},
		RouterVar: "r",
	}

	resolvePrefixes(result)

	if result.Procedures[0].Name != "hello" {
		t.Errorf("expected 'hello' unchanged, got %q", result.Procedures[0].Name)
	}
}

func TestGenerateFromProcedures_Subscription(t *testing.T) {
	procs := []ProcedureInfo{
		{Name: "onMessage", Type: "subscription"},
	}

	output := GenerateFromProcedures(procs, "AppRouter")

	if !strings.Contains(output, `type: "subscription"`) {
		t.Error("expected subscription type")
	}
	if !strings.Contains(output, "onMessage:") {
		t.Error("expected procedure name 'onMessage'")
	}
}

func TestGetProcType_Subscription(t *testing.T) {
	if got := getProcType("Subscription"); got != "subscription" {
		t.Errorf("getProcType(\"Subscription\") = %q, want \"subscription\"", got)
	}
}

func TestExtractEnumsFromAST_StringEnum(t *testing.T) {
	src := `package example

type Status string

const (
	StatusActive   Status = "active"
	StatusInactive Status = "inactive"
)
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}

	enums := extractEnumsFromAST(file)
	if len(enums) != 1 {
		t.Fatalf("expected 1 enum, got %d", len(enums))
	}
	if enums[0].TypeName != "Status" {
		t.Errorf("expected TypeName 'Status', got %q", enums[0].TypeName)
	}
	if !enums[0].IsString {
		t.Error("expected IsString to be true")
	}
	if len(enums[0].Values) != 2 {
		t.Fatalf("expected 2 values, got %d", len(enums[0].Values))
	}
	if enums[0].Values[0] != "active" || enums[0].Values[1] != "inactive" {
		t.Errorf("unexpected values: %v", enums[0].Values)
	}
}

func TestExtractEnumsFromAST_IotaEnum(t *testing.T) {
	src := `package example

type Priority int

const (
	PriorityLow Priority = iota
	PriorityMedium
	PriorityHigh
)
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}

	enums := extractEnumsFromAST(file)
	if len(enums) != 1 {
		t.Fatalf("expected 1 enum, got %d", len(enums))
	}
	if enums[0].TypeName != "Priority" {
		t.Errorf("expected TypeName 'Priority', got %q", enums[0].TypeName)
	}
	if enums[0].IsString {
		t.Error("expected IsString to be false")
	}
	if len(enums[0].Values) != 3 {
		t.Fatalf("expected 3 values, got %d", len(enums[0].Values))
	}
	if enums[0].Values[0] != "0" || enums[0].Values[1] != "1" || enums[0].Values[2] != "2" {
		t.Errorf("unexpected values: %v", enums[0].Values)
	}
}

func TestExtractEnumsFromAST_NoEnums(t *testing.T) {
	src := `package example

const MaxRetries = 3
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}

	enums := extractEnumsFromAST(file)
	if len(enums) != 0 {
		t.Errorf("expected no enums, got %d", len(enums))
	}
}

func TestGenerateTS(t *testing.T) {
	result := &ParseResult{
		Procedures: []ProcedureInfo{
			{Name: "getUser", Type: "query"},
			{Name: "createUser", Type: "mutation"},
			{Name: "onMessage", Type: "subscription"},
		},
	}
	opts := GenerateOptions{
		RouterName: "AppRouter",
		OutputPath: "client.ts",
	}
	output := generateTS(result, opts)

	if !strings.Contains(output, "DO NOT EDIT") {
		t.Error("expected generated header comment")
	}
	if !strings.Contains(output, `export type { AppRouter }`) {
		t.Error("expected AppRouter re-export")
	}
	if !strings.Contains(output, `"getUser": { type: "query" as const }`) {
		t.Error("expected getUser procedure entry")
	}
	if !strings.Contains(output, `"createUser": { type: "mutation" as const }`) {
		t.Error("expected createUser procedure entry")
	}
	if !strings.Contains(output, `"onMessage": { type: "subscription" as const }`) {
		t.Error("expected onMessage procedure entry")
	}
	if !strings.Contains(output, "export type ProcedureName = keyof typeof procedures") {
		t.Error("expected ProcedureName type export")
	}
}

func TestParseDirSimple(t *testing.T) {
	result, err := ParseDir("testdata/simple")
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Procedures) != 2 {
		t.Fatalf("expected 2 procedures, got %d: %+v", len(result.Procedures), result.Procedures)
	}

	names := make(map[string]string)
	for _, p := range result.Procedures {
		names[p.Name] = p.Type
	}

	if names["ping"] != "query" {
		t.Errorf("expected ping=query, got %v", names["ping"])
	}
	if names["createItem"] != "mutation" {
		t.Errorf("expected createItem=mutation, got %v", names["createItem"])
	}
}

func TestParseDirWithEnums(t *testing.T) {
	result, err := ParseDir("testdata/with_enums")
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Procedures) != 1 {
		t.Fatalf("expected 1 procedure, got %d", len(result.Procedures))
	}
	if result.Procedures[0].Name != "getStatus" {
		t.Errorf("expected getStatus, got %s", result.Procedures[0].Name)
	}

	if len(result.Enums) != 1 {
		t.Fatalf("expected 1 enum, got %d", len(result.Enums))
	}
	if result.Enums[0].TypeName != "Status" {
		t.Errorf("expected Status enum, got %s", result.Enums[0].TypeName)
	}
}

func TestParseDirNested(t *testing.T) {
	result, err := ParseDir("testdata/nested")
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Procedures) != 1 {
		t.Fatalf("expected 1 procedure, got %d", len(result.Procedures))
	}
	// After prefix resolution, should be "admin.listUsers"
	if result.Procedures[0].Name != "admin.listUsers" {
		t.Errorf("expected 'admin.listUsers', got %q", result.Procedures[0].Name)
	}
}

func TestParseDirNonexistent(t *testing.T) {
	_, err := ParseDir("testdata/nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent directory")
	}
}

func TestGenerate_DTS(t *testing.T) {
	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "out", "router.d.ts")

	err := Generate(GenerateOptions{
		SourcePath: "testdata/simple",
		OutputPath: outPath,
		Format:     "dts",
	})
	if err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}

	output := string(content)
	if !strings.Contains(output, "DO NOT EDIT") {
		t.Error("expected header comment")
	}
	if !strings.Contains(output, "ping:") {
		t.Error("expected ping procedure")
	}
	if !strings.Contains(output, "createItem:") {
		t.Error("expected createItem procedure")
	}
	if !strings.Contains(output, "AppRouter") {
		t.Error("expected AppRouter type")
	}
}

func TestGenerate_TS(t *testing.T) {
	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "client.ts")

	err := Generate(GenerateOptions{
		SourcePath: "testdata/simple",
		OutputPath: outPath,
		Format:     "ts",
	})
	if err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}

	output := string(content)
	if !strings.Contains(output, "procedures") {
		t.Error("expected procedures const")
	}
	if !strings.Contains(output, `"ping"`) {
		t.Error("expected ping in TS output")
	}
}

func TestGenerate_NoProcedures(t *testing.T) {
	tmpDir := t.TempDir()

	// Create an empty Go file with no procedures
	emptyDir := filepath.Join(tmpDir, "empty")
	os.MkdirAll(emptyDir, 0o755)
	os.WriteFile(filepath.Join(emptyDir, "main.go"), []byte("package empty\n"), 0o644)

	err := Generate(GenerateOptions{
		SourcePath: emptyDir,
		OutputPath: filepath.Join(tmpDir, "out.d.ts"),
	})
	if err == nil {
		t.Fatal("expected error for no procedures")
	}
	if !strings.Contains(err.Error(), "no tRPC procedures found") {
		t.Errorf("expected 'no tRPC procedures found' error, got: %v", err)
	}
}

func TestGenerateTS_SortedOutput(t *testing.T) {
	result := &ParseResult{
		Procedures: []ProcedureInfo{
			{Name: "zebra", Type: "query"},
			{Name: "alpha", Type: "mutation"},
		},
	}
	opts := GenerateOptions{
		RouterName: "AppRouter",
		OutputPath: "client.ts",
	}
	output := generateTS(result, opts)

	alphaIdx := strings.Index(output, `"alpha"`)
	zebraIdx := strings.Index(output, `"zebra"`)
	if alphaIdx > zebraIdx {
		t.Error("expected procedures to be sorted alphabetically")
	}
}
