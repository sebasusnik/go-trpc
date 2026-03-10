package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize go-trpc in the current project",
	Long:  "Creates gotrpc.json config and scaffolds the TypeScript client setup.",
	RunE:  runInit,
}

func init() {
	initCmd.Flags().Bool("ws", false, "scaffold with WebSocket support (splitLink + wsLink for subscriptions)")
}

const generatedGitignore = `*
!.gitignore
`

func trpcHTTPTemplate(routerName string) string {
	return fmt.Sprintf(`import { createTRPCClient, httpLink } from "@trpc/client";
import type { %s } from "./generated/router";

export const trpc = createTRPCClient<%s>({
  links: [httpLink({ url: "/trpc" })],
});
`, routerName, routerName)
}

func trpcWSTemplate(routerName string) string {
	return fmt.Sprintf(`import {
  createTRPCClient,
  httpLink,
  splitLink,
  wsLink,
} from "@trpc/client";
import type { %s } from "./generated/router";

const url = window.location.origin + "/trpc";
const wsUrl = url.replace(/^http/, "ws");

export const trpc = createTRPCClient<%s>({
  links: [
    splitLink({
      condition: (op) => op.type === "subscription",
      true: wsLink({ url: wsUrl }),
      false: httpLink({ url }),
    }),
  ],
});
`, routerName, routerName)
}

func runInit(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	useWS, _ := cmd.Flags().GetBool("ws")

	// Detect project structure
	source := detectGoSource(cwd)
	webDir := detectWebDir(cwd)
	srcDir := detectSrcDir(cwd, webDir)

	cfg := GoTRPCConfig{
		Source: "./" + source,
		Output: "./" + filepath.Join(webDir, srcDir, "generated/router.d.ts"),
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
	generatedDir := filepath.Join(cwd, webDir, srcDir, "generated")
	if err := os.MkdirAll(generatedDir, 0o755); err != nil {
		return fmt.Errorf("failed to create generated dir: %w", err)
	}

	gitignorePath := filepath.Join(generatedDir, ".gitignore")
	if _, err := os.Stat(gitignorePath); err != nil {
		if err := os.WriteFile(gitignorePath, []byte(generatedGitignore), 0o644); err != nil {
			return fmt.Errorf("failed to write .gitignore: %w", err)
		}
		fmt.Printf("Created %s\n", filepath.Join(webDir, srcDir, "generated/.gitignore"))
	}

	// Create trpc.ts if it doesn't exist
	trpcPath := filepath.Join(cwd, webDir, srcDir, "trpc.ts")
	relTrpcPath := filepath.Join(webDir, srcDir, "trpc.ts")
	if _, err := os.Stat(trpcPath); err == nil {
		fmt.Printf("%s already exists, skipping\n", relTrpcPath)
	} else {
		var template string
		if useWS {
			template = trpcWSTemplate(cfg.Router)
		} else {
			template = trpcHTTPTemplate(cfg.Router)
		}
		if err := os.WriteFile(trpcPath, []byte(template), 0o644); err != nil {
			return fmt.Errorf("failed to write trpc.ts: %w", err)
		}
		fmt.Printf("Created %s\n", relTrpcPath)
	}

	fmt.Println("\nNext steps:")
	fmt.Println("  1. npm install @trpc/client @trpc/server")
	fmt.Println("  2. gotrpc generate")
	fmt.Printf("  3. Import { trpc } from \"./%s\" in your components\n", strings.TrimSuffix(filepath.Base(relTrpcPath), ".ts"))

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

// detectSrcDir checks if the web directory uses a src/ subdirectory.
func detectSrcDir(projectDir, webDir string) string {
	srcPath := filepath.Join(projectDir, webDir, "src")
	if info, err := os.Stat(srcPath); err == nil && info.IsDir() {
		return "src"
	}
	return ""
}
