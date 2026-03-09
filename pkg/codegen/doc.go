// Package codegen generates TypeScript type definitions from Go source code.
//
// It scans Go packages for tRPC procedure registrations (Query, Mutation,
// Subscription) and produces .d.ts files compatible with @trpc/client.
//
// # Parsing Strategies
//
// The package uses two parsing strategies with automatic fallback:
//
//   - [ParsePackage] uses golang.org/x/tools/go/packages for full type
//     resolution. This is the preferred path and produces the most accurate
//     TypeScript types, including types from external packages.
//
//   - [ParseDir] uses go/parser for AST-only extraction without type
//     checking. This fallback handles cases where the Go toolchain cannot
//     fully resolve types (e.g., missing dependencies). It extracts type
//     names as strings and resolves struct definitions from AST.
//
// Both strategies scan for gotrpc.Query(), gotrpc.Mutation(), and
// gotrpc.Subscription() call expressions in the AST, extract the procedure
// name (second argument) and handler type information (third argument),
// and resolve namespace prefixes from Router.Merge() calls.
//
// # Type Mapping
//
// Go types are mapped to TypeScript via [TypeMapper]:
//
//	string           → string
//	int, float64     → number
//	bool             → boolean
//	[]T              → T[]
//	map[K]V          → Record<K, V>
//	*T               → T | null
//	struct{}         → void
//	time.Time        → string (ISO 8601)
//	enum patterns    → union types ("active" | "inactive")
//
// # Output
//
// [Generate] produces either .d.ts (type declarations) or .ts (with
// runtime metadata). The output includes an AppRouter interface with
// nested namespaces matching the procedure hierarchy.
package codegen
