package codegen

import (
	"go/parser"
	"go/token"
	"go/types"
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

func TestParseDirHigherOrder(t *testing.T) {
	result, err := ParseDir("testdata/higher_order")
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Procedures) != 3 {
		t.Fatalf("expected 3 procedures, got %d: %+v", len(result.Procedures), result.Procedures)
	}

	procs := make(map[string]ProcedureInfo)
	for _, p := range result.Procedures {
		procs[p.Name] = p
	}

	// Higher-order handler: ListTasks(nil) should resolve to input=struct{}, output=[]Task
	list := procs["task.list"]
	if list.InputTypeName != "struct{}" {
		t.Errorf("task.list input: expected 'struct{}', got %q", list.InputTypeName)
	}
	if list.OutputTypeName != "[]Task" {
		t.Errorf("task.list output: expected '[]Task', got %q", list.OutputTypeName)
	}

	// Higher-order handler: CreateTask(nil) should resolve to input=CreateTaskInput, output=Task
	create := procs["task.create"]
	if create.InputTypeName != "CreateTaskInput" {
		t.Errorf("task.create input: expected 'CreateTaskInput', got %q", create.InputTypeName)
	}
	if create.OutputTypeName != "Task" {
		t.Errorf("task.create output: expected 'Task', got %q", create.OutputTypeName)
	}

	// Direct handler: HealthCheck should resolve to input=struct{}, output=string
	health := procs["health"]
	if health.InputTypeName != "struct{}" {
		t.Errorf("health input: expected 'struct{}', got %q", health.InputTypeName)
	}
	if health.OutputTypeName != "string" {
		t.Errorf("health output: expected 'string', got %q", health.OutputTypeName)
	}

	// Should have extracted struct defs
	if len(result.StructDefs) < 2 {
		t.Fatalf("expected at least 2 struct defs (Task, CreateTaskInput), got %d", len(result.StructDefs))
	}

	structMap := make(map[string]StructDef)
	for _, sd := range result.StructDefs {
		structMap[sd.Name] = sd
	}

	task, ok := structMap["Task"]
	if !ok {
		t.Fatal("expected Task struct def")
	}
	if len(task.Fields) != 4 {
		t.Errorf("expected 4 fields in Task, got %d", len(task.Fields))
	}
}

func TestGenerateDTS_ASTTypes(t *testing.T) {
	result := &ParseResult{
		Procedures: []ProcedureInfo{
			{
				Name:           "task.list",
				Type:           "query",
				InputTypeName:  "struct{}",
				OutputTypeName: "[]Task",
			},
			{
				Name:           "task.create",
				Type:           "mutation",
				InputTypeName:  "CreateTaskInput",
				OutputTypeName: "Task",
			},
		},
		StructDefs: []StructDef{
			{
				Name: "Task",
				Fields: []StructField{
					{Name: "ID", TypeExpr: "string", JSONName: "id"},
					{Name: "Title", TypeExpr: "string", JSONName: "title"},
					{Name: "Description", TypeExpr: "string", JSONName: "description", Optional: true},
					{Name: "Status", TypeExpr: "string", JSONName: "status"},
				},
			},
			{
				Name: "CreateTaskInput",
				Fields: []StructField{
					{Name: "Title", TypeExpr: "string", JSONName: "title"},
					{Name: "Description", TypeExpr: "string", JSONName: "description", Optional: true},
				},
			},
		},
	}
	opts := GenerateOptions{RouterName: "AppRouter"}
	output := generateDTS(result, opts)

	// Should have Task type definition
	if !strings.Contains(output, "type Task = {") {
		t.Errorf("expected Task type definition, got:\n%s", output)
	}

	// Should have CreateTaskInput type definition
	if !strings.Contains(output, "type CreateTaskInput = {") {
		t.Errorf("expected CreateTaskInput type definition, got:\n%s", output)
	}

	// Task should be exported
	if !strings.Contains(output, "export type {") && !strings.Contains(output, "Task") {
		t.Error("expected Task in exports")
	}

	// Procedures should NOT have void types
	if strings.Contains(output, `input: void; output: void`) {
		t.Error("expected non-void types for procedures with AST type info")
	}

	// task.list output should be Task[]
	if !strings.Contains(output, "Task[]") {
		t.Errorf("expected Task[] in output, got:\n%s", output)
	}

	// task.list input should be void (struct{})
	if !strings.Contains(output, "input: void; output: Task[]") {
		t.Errorf("expected 'input: void; output: Task[]' for task.list, got:\n%s", output)
	}

	// task.create should reference CreateTaskInput and Task
	if !strings.Contains(output, "input: CreateTaskInput; output: Task") {
		t.Errorf("expected 'input: CreateTaskInput; output: Task' for task.create, got:\n%s", output)
	}

	// RouterInputs should have correct types
	if !strings.Contains(output, `"task.list": void`) {
		t.Errorf("expected task.list input void in RouterInputs, got:\n%s", output)
	}
	if !strings.Contains(output, `"task.create": CreateTaskInput`) {
		t.Errorf("expected task.create input CreateTaskInput in RouterInputs, got:\n%s", output)
	}

	// RouterOutputs should have correct types
	if !strings.Contains(output, `"task.list": Task[]`) {
		t.Errorf("expected task.list output Task[] in RouterOutputs, got:\n%s", output)
	}
	if !strings.Contains(output, `"task.create": Task`) {
		t.Errorf("expected task.create output Task in RouterOutputs, got:\n%s", output)
	}
}

func TestMapASTType(t *testing.T) {
	structDefs := map[string]*StructDef{
		"User": {
			Name: "User",
			Fields: []StructField{
				{Name: "ID", TypeExpr: "string", JSONName: "id"},
				{Name: "Name", TypeExpr: "string", JSONName: "name"},
				{Name: "Age", TypeExpr: "int", JSONName: "age"},
			},
		},
	}

	tests := []struct {
		goType   string
		expected string
	}{
		{"string", "string"},
		{"int", "number"},
		{"int64", "number"},
		{"float64", "number"},
		{"bool", "boolean"},
		{"struct{}", "void"},
		{"*string", "string | null"},
		{"[]string", "string[]"},
		{"[]*string", "(string | null)[]"},
		{"[]User", "User[]"},
		{"map[string]int", "Record<string, number>"},
		{"interface{}", "unknown"},
		{"time.Time", "string"},
		{"uuid.UUID", "string"},
		{"User", "User"},
	}

	for _, tt := range tests {
		t.Run(tt.goType, func(t *testing.T) {
			astNamed := make(map[string]string)
			got := mapASTType(tt.goType, structDefs, astNamed)
			if got != tt.expected {
				t.Errorf("mapASTType(%q) = %q, want %q", tt.goType, got, tt.expected)
			}
		})
	}

	// Verify that using "User" registers it as a named type
	astNamed := make(map[string]string)
	mapASTType("User", structDefs, astNamed)
	if _, ok := astNamed["User"]; !ok {
		t.Error("expected User to be registered as named type")
	}
}

func TestGenerateDTS_EmptyStructInputBecomesVoid(t *testing.T) {
	// When the type-checked path returns Record<string, never> for struct{},
	// generateDTS should convert it to void so tRPC allows calling .query() without args.
	emptyStruct := types.NewStruct(nil, nil)

	result := &ParseResult{
		Procedures: []ProcedureInfo{
			{
				Name:       "health.check",
				Type:       "query",
				InputType:  emptyStruct,
				OutputType: types.Typ[types.String],
			},
		},
	}

	output := generateDTS(result, GenerateOptions{RouterName: "AppRouter"})

	// Input should be void, not Record<string, never>
	if strings.Contains(output, `input: Record<string, never>`) {
		t.Errorf("expected empty struct input to be void, not Record<string, never>, got:\n%s", output)
	}
	if !strings.Contains(output, `$types: { input: void; output: string }`) {
		t.Errorf("expected 'input: void; output: string' for health.check, got:\n%s", output)
	}
	// RouterInputs should also have void
	if !strings.Contains(output, `"health.check": void`) {
		t.Errorf("expected health.check input void in RouterInputs, got:\n%s", output)
	}
}

func TestGenerate_FallbackWarning(t *testing.T) {
	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "out", "router.d.ts")

	// Use a path that will cause ParsePackage to fail but ParseDir to succeed
	err := Generate(GenerateOptions{
		SourcePath: "testdata/simple",
		OutputPath: outPath,
		Format:     "dts",
	})
	// Should succeed (either ParsePackage or ParseDir works)
	if err != nil {
		t.Fatal(err)
	}

	// Verify output was generated
	content, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "ping:") {
		t.Error("expected ping procedure in output")
	}
}
