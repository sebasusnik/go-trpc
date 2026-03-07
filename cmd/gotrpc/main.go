package main

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/sebasusnik/go-trpc/pkg/codegen"
	"github.com/spf13/cobra"
)

func main() {
	var outputPath string
	var routerName string
	var watch bool

	rootCmd := &cobra.Command{
		Use:   "gotrpc",
		Short: "go-trpc CLI - tRPC code generation for Go",
	}

	generateCmd := &cobra.Command{
		Use:   "generate [path]",
		Short: "Generate TypeScript types from Go tRPC definitions",
		Long: `Parses Go source files to find gotrpc.Query() and gotrpc.Mutation() calls,
extracts the input/output types, and generates a .d.ts file compatible with @trpc/client.

If no path argument is given, reads configuration from gotrpc.json in the current directory.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var sourcePath string

			if len(args) == 1 {
				// Explicit path provided
				sourcePath = args[0]
			} else {
				// Try to load config
				cwd, _ := os.Getwd()
				cfg, err := LoadConfig(cwd)
				if err != nil {
					return fmt.Errorf("no path argument and %v\nUsage: gotrpc generate [path] or create a gotrpc.json", err)
				}
				sourcePath = cfg.Source
				if outputPath == "" {
					outputPath = cfg.Output
				}
				if routerName == "" {
					routerName = cfg.Router
				}
			}

			if outputPath == "" {
				outputPath = "./generated/router.d.ts"
			}

			if routerName == "" {
				routerName = "AppRouter"
			}

			opts := codegen.GenerateOptions{
				OutputPath: outputPath,
				RouterName: routerName,
				SourcePath: sourcePath,
			}

			fmt.Printf("Parsing %s...\n", sourcePath)

			err := codegen.Generate(opts)
			if err != nil {
				if !watch {
					return fmt.Errorf("generation failed: %w", err)
				}
				fmt.Fprintf(os.Stderr, "Generation failed: %v\n", err)
			} else {
				fmt.Printf("Generated %s\n", outputPath)
			}

			if watch {
				return watchAndRegenerate(sourcePath, opts)
			}

			return nil
		},
	}

	generateCmd.Flags().StringVarP(&outputPath, "output", "o", "", "output path for the .d.ts file")
	generateCmd.Flags().StringVarP(&routerName, "router", "r", "", "name of the router type (default: AppRouter)")
	generateCmd.Flags().BoolVarP(&watch, "watch", "w", false, "watch for .go file changes and regenerate")

	rootCmd.AddCommand(generateCmd)
	rootCmd.AddCommand(initCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func watchAndRegenerate(sourcePath string, opts codegen.GenerateOptions) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("failed to create watcher: %w", err)
	}
	defer watcher.Close()

	// Recursively add all directories under sourcePath
	err = filepath.Walk(sourcePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return watcher.Add(path)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to watch %s: %w", sourcePath, err)
	}

	fmt.Printf("Watching %s for changes...\n", sourcePath)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	var debounce *time.Timer

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}
			if !strings.HasSuffix(event.Name, ".go") {
				continue
			}
			if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove|fsnotify.Rename) == 0 {
				continue
			}

			// Debounce: wait 500ms for rapid successive events
			if debounce != nil {
				debounce.Stop()
			}
			debounce = time.AfterFunc(500*time.Millisecond, func() {
				fmt.Printf("\nFile changed: %s\nRegenerating...\n", filepath.Base(event.Name))
				if err := codegen.Generate(opts); err != nil {
					fmt.Fprintf(os.Stderr, "Generation failed: %v\n", err)
				} else {
					fmt.Printf("Generated %s\n", opts.OutputPath)
				}
			})

		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			fmt.Fprintf(os.Stderr, "Watch error: %v\n", err)

		case <-sig:
			fmt.Println("\nStopping watcher...")
			return nil
		}
	}
}
