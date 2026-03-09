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

// exprToVarName extracts a variable name from an expression.
func exprToVarName(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	default:
		return ""
	}
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
