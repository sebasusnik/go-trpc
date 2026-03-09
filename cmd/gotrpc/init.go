package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize go-trpc in the current project",
	Long:  "Creates gotrpc.json config and scaffolds the TypeScript client setup.",
	RunE:  runInit,
}

const trpcTemplate = `import { createGoTRPCClient } from "@go-trpc/client";
import type { AppRouter } from "./generated/router";

const baseUrl = "/trpc";

export const trpc = createGoTRPCClient<AppRouter>({ url: baseUrl });
`

const generatedGitignore = `*
!.gitignore
`

func runInit(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	// Detect project structure
	source := detectGoSource(cwd)
	webDir := detectWebDir(cwd)

	cfg := GoTRPCConfig{
		Source: "./" + source,
		Output: "./" + filepath.Join(webDir, "src/generated/router.d.ts"),
		Router: "AppRouter",
	}

	// Write gotrpc.json
	configPath := filepath.Join(cwd, configFileName)
	if _, err := os.Stat(configPath); err == nil {
		fmt.Printf("%s already exists, skipping\n", configFileName)
	} else {
		data, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal config: %w", err)
		}
		data = append(data, '\n')
		if err := os.WriteFile(configPath, data, 0o644); err != nil {
			return fmt.Errorf("failed to write %s: %w", configFileName, err)
		}
		fmt.Printf("Created %s\n", configFileName)
	}

	// Create generated directory with .gitignore
	generatedDir := filepath.Join(cwd, webDir, "src/generated")
	if err := os.MkdirAll(generatedDir, 0o755); err != nil {
		return fmt.Errorf("failed to create generated dir: %w", err)
	}

	gitignorePath := filepath.Join(generatedDir, ".gitignore")
	if _, err := os.Stat(gitignorePath); err != nil {
		if err := os.WriteFile(gitignorePath, []byte(generatedGitignore), 0o644); err != nil {
			return fmt.Errorf("failed to write .gitignore: %w", err)
		}
		fmt.Printf("Created %s\n", filepath.Join(webDir, "src/generated/.gitignore"))
	}

	// Create trpc.ts if it doesn't exist
	trpcPath := filepath.Join(cwd, webDir, "src/trpc.ts")
	if _, err := os.Stat(trpcPath); err == nil {
		fmt.Printf("%s already exists, skipping\n", filepath.Join(webDir, "src/trpc.ts"))
	} else {
		if err := os.WriteFile(trpcPath, []byte(trpcTemplate), 0o644); err != nil {
			return fmt.Errorf("failed to write trpc.ts: %w", err)
		}
		fmt.Printf("Created %s\n", filepath.Join(webDir, "src/trpc.ts"))
	}

	fmt.Println("\nNext steps:")
	fmt.Println("  1. npm install @go-trpc/client")
	fmt.Println("  2. gotrpc generate")
	fmt.Println("  3. Import { trpc } from \"./trpc\" in your components")

	return nil
}

// detectGoSource looks for common Go API directory names.
func detectGoSource(dir string) string {
	candidates := []string{"api", "server", "backend", "cmd"}
	for _, c := range candidates {
		if info, err := os.Stat(filepath.Join(dir, c)); err == nil && info.IsDir() {
			// Check for go.mod in the candidate or its parent
			if _, err := os.Stat(filepath.Join(dir, c, "go.mod")); err == nil {
				return c
			}
			if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
				return c
			}
		}
	}
	return "api"
}

// detectWebDir looks for common web/frontend directory names.
func detectWebDir(dir string) string {
	candidates := []string{"web", "frontend", "client", "app"}
	for _, c := range candidates {
		if info, err := os.Stat(filepath.Join(dir, c)); err == nil && info.IsDir() {
			if _, err := os.Stat(filepath.Join(dir, c, "package.json")); err == nil {
				return c
			}
		}
	}
	return "web"
}
