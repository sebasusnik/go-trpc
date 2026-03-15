package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var createCmd = &cobra.Command{
	Use:   "create <project-name>",
	Short: "Create a full-stack go-trpc project (the Go T3 stack)",
	Long: `Scaffolds a complete full-stack project with:
  • Go backend with go-trpc, structured handlers, and nethttp adapter
  • React + Vite + TypeScript frontend with Tailwind CSS
  • tRPC client pre-configured with type generation
  • Makefile with dev/build/generate commands
  • Docker setup for development and production

Optional features:
  --ws     WebSocket subscriptions (splitLink + wsLink)
  --auth   Authentication middleware with JWT helpers
  --db     Database layer with sqlc (PostgreSQL)`,
	Args: cobra.ExactArgs(1),
	RunE: runCreate,
}

func init() {
	createCmd.Flags().Bool("ws", false, "include WebSocket subscription support")
	createCmd.Flags().Bool("auth", false, "include authentication middleware and helpers")
	createCmd.Flags().Bool("db", false, "include database layer with sqlc (PostgreSQL)")
}

type createOptions struct {
	Name string
	WS   bool
	Auth bool
	DB   bool
}

func runCreate(cmd *cobra.Command, args []string) error {
	name := args[0]
	if name == "" {
		return fmt.Errorf("project name is required")
	}
	if strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return fmt.Errorf("project name cannot contain path separators")
	}

	ws, _ := cmd.Flags().GetBool("ws")
	auth, _ := cmd.Flags().GetBool("auth")
	db, _ := cmd.Flags().GetBool("db")

	opts := createOptions{
		Name: name,
		WS:   ws,
		Auth: auth,
		DB:   db,
	}

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	projectDir := filepath.Join(cwd, name)
	if _, err := os.Stat(projectDir); err == nil {
		return fmt.Errorf("directory %q already exists", name)
	}

	green := "\033[32m"
	bold := "\033[1m"
	dim := "\033[2m"
	cyan := "\033[36m"
	reset := "\033[0m"

	fmt.Println()
	fmt.Printf("  %s%sCreating go-trpc project: %s%s\n", bold, cyan, name, reset)
	fmt.Println()

	features := []string{"go-trpc", "React", "Vite", "TypeScript", "Tailwind CSS"}
	if ws {
		features = append(features, "WebSocket")
	}
	if auth {
		features = append(features, "Auth")
	}
	if db {
		features = append(features, "sqlc + PostgreSQL")
	}
	fmt.Printf("  %sFeatures:%s %s\n\n", dim, reset, strings.Join(features, ", "))

	// Create directory structure
	dirs := []string{
		"api",
		"api/handlers",
		"web/src",
		"web/src/components",
		"web/src/generated",
		"web/public",
	}
	if auth {
		dirs = append(dirs, "api/auth")
	}
	if db {
		dirs = append(dirs, "api/db", "api/db/queries", "api/db/migrations")
	}

	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(projectDir, d), 0o755); err != nil {
			return fmt.Errorf("failed to create %s: %w", d, err)
		}
	}

	// Write all files
	files := generateProjectFiles(opts)

	for path, content := range files {
		fullPath := filepath.Join(projectDir, path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			return fmt.Errorf("failed to create directory for %s: %w", path, err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
			return fmt.Errorf("failed to write %s: %w", path, err)
		}
		fmt.Printf("  %s✓%s %s\n", green, reset, path)
	}

	// Write gotrpc.json
	cfg := GoTRPCConfig{
		Source: "./api",
		Output: "./web/src/generated/router.d.ts",
		Router: "AppRouter",
	}
	cfgData, _ := json.MarshalIndent(cfg, "", "  ")
	cfgData = append(cfgData, '\n')
	cfgPath := filepath.Join(projectDir, configFileName)
	if err := os.WriteFile(cfgPath, cfgData, 0o644); err != nil {
		return fmt.Errorf("failed to write gotrpc.json: %w", err)
	}
	fmt.Printf("  %s✓%s %s\n", green, reset, configFileName)

	// Print summary
	fmt.Printf("\n  %sProject structure:%s\n", bold, reset)
	fmt.Printf("    %s%s/%s\n", cyan, name, reset)
	fmt.Printf("    ├── api/\n")
	fmt.Printf("    │   ├── main.go              %s← Go entrypoint%s\n", dim, reset)
	fmt.Printf("    │   └── handlers/\n")
	fmt.Printf("    │       └── user.go           %s← sample handlers%s\n", dim, reset)
	if auth {
		fmt.Printf("    │   └── auth/\n")
		fmt.Printf("    │       └── middleware.go     %s← JWT auth middleware%s\n", dim, reset)
	}
	if db {
		fmt.Printf("    │   └── db/\n")
		fmt.Printf("    │       ├── queries/          %s← sqlc SQL queries%s\n", dim, reset)
		fmt.Printf("    │       └── migrations/       %s← schema migrations%s\n", dim, reset)
	}
	fmt.Printf("    ├── web/\n")
	fmt.Printf("    │   ├── src/\n")
	fmt.Printf("    │   │   ├── App.tsx           %s← React app%s\n", dim, reset)
	fmt.Printf("    │   │   ├── trpc.ts           %s← tRPC client%s\n", dim, reset)
	fmt.Printf("    │   │   └── generated/        %s← auto-generated types%s\n", dim, reset)
	fmt.Printf("    │   └── package.json\n")
	fmt.Printf("    ├── Makefile                  %s← dev/build/generate commands%s\n", dim, reset)
	fmt.Printf("    ├── Dockerfile\n")
	fmt.Printf("    └── gotrpc.json\n")

	fmt.Printf("\n  %sGet started:%s\n", bold, reset)
	fmt.Printf("    %s$%s cd %s\n", dim, reset, name)
	fmt.Printf("    %s$%s make setup     %s← install Go + npm dependencies%s\n", dim, reset, dim, reset)
	fmt.Printf("    %s$%s make dev       %s← start backend + frontend + codegen watcher%s\n", dim, reset, dim, reset)
	fmt.Println()

	return nil
}

func generateProjectFiles(opts createOptions) map[string]string {
	files := make(map[string]string)

	// === Go backend ===
	files["api/go.mod"] = goModTemplate(opts)
	files["api/main.go"] = goMainTemplate(opts)
	files["api/handlers/user.go"] = goUserHandlerTemplate(opts)

	if opts.Auth {
		files["api/auth/middleware.go"] = goAuthMiddlewareTemplate(opts)
	}

	if opts.DB {
		files["api/db/queries/users.sql"] = sqlcQueriesTemplate()
		files["api/db/migrations/001_init.sql"] = sqlcMigrationTemplate()
		files["api/db/sqlc.yaml"] = sqlcConfigTemplate()
	}

	// === Frontend ===
	files["web/package.json"] = packageJSONTemplate(opts)
	files["web/tsconfig.json"] = tsconfigTemplate()
	files["web/vite.config.ts"] = viteConfigTemplate()
	files["web/index.html"] = indexHTMLTemplate(opts)
	files["web/src/main.tsx"] = mainTSXTemplate()
	files["web/src/App.tsx"] = appTSXTemplate(opts)
	files["web/src/trpc.ts"] = trpcClientTemplate(opts)
	files["web/src/styles.css"] = stylesTemplate()
	files["web/src/components/UserCard.tsx"] = userCardTemplate()
	files["web/src/generated/.gitignore"] = generatedGitignore
	files["web/src/vite-env.d.ts"] = "/// <reference types=\"vite/client\" />\n"
	files["web/postcss.config.js"] = postcssConfigTemplate()
	files["web/tailwind.config.js"] = tailwindConfigTemplate()

	// === Root ===
	files["Makefile"] = makefileTemplate(opts)
	files["Dockerfile"] = dockerfileTemplate(opts)
	files[".gitignore"] = rootGitignoreTemplate()
	files["docker-compose.yml"] = dockerComposeTemplate(opts)

	return files
}
