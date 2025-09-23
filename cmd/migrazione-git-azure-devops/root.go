package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// Normalizes old multi-letter short flags into long Cobra-compatible flags.
func normalizeLegacyArgs(args []string) []string {
	out := make([]string, 0, len(args))
	m := map[string]string{
		"-so": "--src-org",
		"-sp": "--src-project",
		"-do": "--dst-org",
		"-dp": "--dst-project",
		"-rl": "--repo-list",
		"-fp": "--force-push",
	}
	for i := 0; i < len(args); i++ {
		if repl, ok := m[args[i]]; ok {
			out = append(out, repl)
			continue
		}
		out = append(out, args[i])
	}
	return out
}

// Execute configures Cobra and starts the root command.
func Execute() {
	// Support for old multi-letter flags (e.g.: -so, -sp, ...).
	os.Args = normalizeLegacyArgs(os.Args)

	var cfg Config
	var repoListPath string

	rootCmd := &cobra.Command{
		Use:   prog(),
		Short: "Git repository migration between Azure DevOps projects/organizations",
		Long:  "Migrates Git repositories between Azure DevOps projects/organizations with wizard or non-interactive mode, dry-run and mirror push.",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Version
			if cfg.ShowVersion {
				printVersion()
				return nil
			}

			// PAT from env
			cfg.SrcPAT = strings.TrimSpace(os.Getenv("SRC_PAT"))
			cfg.DstPAT = strings.TrimSpace(os.Getenv("DST_PAT"))

			if cfg.Trace {
				fmt.Fprintln(os.Stderr, "[TRACE] Trace enabled")
			}

			// Minimal validations
			if cfg.SrcOrg == "" || cfg.SrcProject == "" {
				return fmt.Errorf("--src-org and --src-project are required")
			}
			if cfg.SrcPAT == "" {
				return fmt.Errorf("SRC_PAT environment variable missing")
			}

			isMigration := !cfg.ListOnly && !cfg.Wizard
			if isMigration {
				if cfg.DstOrg == "" || cfg.DstProject == "" {
					return fmt.Errorf("specify destination (--dst-org, --dst-project) or use --list-repos/--wizard")
				}
				if cfg.DstPAT == "" {
					return fmt.Errorf("DST_PAT environment variable missing for destination")
				}
			}

			// Load repo list from file if provided
			if repoListPath != "" {
				data, err := os.ReadFile(repoListPath)
				if err != nil {
					return fmt.Errorf("error reading --repo-list: %w", err)
				}
				for _, ln := range strings.Split(string(data), "\n") {
					ln = strings.TrimSpace(ln)
					if ln != "" && !strings.HasPrefix(ln, "#") {
						cfg.RepoList = append(cfg.RepoList, ln)
					}
				}
			}

			// Report-path validation
			if len(cfg.ReportFormats) > 0 {
				// Check supported formats
				supported := map[string]bool{"json": true, "html": true}
				for _, f := range cfg.ReportFormats {
					if !supported[strings.ToLower(f)] {
						return fmt.Errorf("unsupported report format: %s (only json, html are allowed)", f)
					}
				}
				if cfg.ReportPath == "" {
					cfg.ReportPath = os.TempDir()
				} else {
					if info, err := os.Stat(cfg.ReportPath); err != nil || !info.IsDir() {
						return fmt.Errorf("--report-path must be an existing directory: %s", cfg.ReportPath)
					}
				}
			}

			// Dispatch
			if cfg.ListOnly {
				return cmdListRepos(cfg)
			}
			if cfg.Wizard {
				return runWizard(cfg)
			}
			return runNonInteractive(cfg)
		},
	}

	// Flag definitions
	rootCmd.Flags().StringVar(&cfg.SrcOrg, "src-org", "", "Source organization (required)")
	rootCmd.Flags().StringVar(&cfg.SrcProject, "src-project", "", "Source project (required)")
	rootCmd.Flags().StringVar(&cfg.DstOrg, "dst-org", "", "Destination organization")
	rootCmd.Flags().StringVar(&cfg.DstProject, "dst-project", "", "Destination project")
	rootCmd.Flags().StringVarP(&cfg.Filter, "filter", "f", "", "Filter repositories with a regex")
	rootCmd.Flags().StringVar(&repoListPath, "repo-list", "", "File with the list of repositories to migrate (one per line)")
	rootCmd.Flags().BoolVar(&cfg.DryRun, "dry-run", false, "Simulate execution without real changes")
	rootCmd.Flags().BoolVar(&cfg.ForcePush, "force-push", false, "Force push if the repository exists in destination")
	rootCmd.Flags().BoolVarP(&cfg.Trace, "trace", "t", false, "Enable detailed trace output")
	rootCmd.Flags().BoolVarP(&cfg.ListOnly, "list-repos", "l", false, "List source repositories and exit")
	rootCmd.Flags().BoolVarP(&cfg.Wizard, "wizard", "w", false, "Start the interactive wizard procedure")
	rootCmd.Flags().BoolVarP(&cfg.ShowVersion, "version", "v", false, "Show program version")
	rootCmd.Flags().StringSliceVar(&cfg.ReportFormats, "report-format", []string{}, "Migration report formats (json, html), comma separated")
	rootCmd.Flags().StringVar(&cfg.ReportPath, "report-path", "", "Directory path to save the report (default: system temp directory)")

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
