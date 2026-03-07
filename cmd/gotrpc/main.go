package main

import (
	"fmt"
	"os"

	"github.com/sebasusnik/go-trpc/pkg/codegen"
	"github.com/spf13/cobra"
)

func main() {
	var outputPath string
	var routerName string

	rootCmd := &cobra.Command{
		Use:   "gotrpc",
		Short: "go-trpc CLI - tRPC code generation for Go",
	}

	generateCmd := &cobra.Command{
		Use:   "generate [path]",
		Short: "Generate TypeScript types from Go tRPC definitions",
		Long: `Parses Go source files to find gotrpc.Query() and gotrpc.Mutation() calls,
extracts the input/output types, and generates a .d.ts file compatible with @trpc/client.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sourcePath := args[0]

			if outputPath == "" {
				outputPath = "./generated/router.d.ts"
			}

			if routerName == "" {
				routerName = "AppRouter"
			}

			fmt.Printf("Parsing %s...\n", sourcePath)

			err := codegen.Generate(codegen.GenerateOptions{
				OutputPath: outputPath,
				RouterName: routerName,
				SourcePath: sourcePath,
			})
			if err != nil {
				return fmt.Errorf("generation failed: %w", err)
			}

			fmt.Printf("Generated %s\n", outputPath)
			return nil
		},
	}

	generateCmd.Flags().StringVarP(&outputPath, "output", "o", "", "output path for the .d.ts file (default: ./generated/router.d.ts)")
	generateCmd.Flags().StringVarP(&routerName, "router", "r", "", "name of the router type (default: AppRouter)")

	rootCmd.AddCommand(generateCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
