package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// Normalizza i vecchi flag short multi-lettera in flag lunghi Cobra-compatibili.
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

// Execute configura Cobra e avvia il comando radice.
func Execute() {
	// Supporto ai vecchi flag multi-lettera (es.: -so, -sp, ...).
	os.Args = normalizeLegacyArgs(os.Args)

	var cfg Config
	var repoListPath string

	rootCmd := &cobra.Command{
		Use:   prog(),
		Short: "Migrazione repository Git tra progetti/organizzazioni Azure DevOps",
		Long:  "Migra repository Git tra progetti/organizzazioni Azure DevOps con modalità wizard o non interattiva, dry-run e push mirror.",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Versione
			if cfg.ShowVersion {
				printVersion()
				return nil
			}

			// PAT da env
			cfg.SrcPAT = strings.TrimSpace(os.Getenv("SRC_PAT"))
			cfg.DstPAT = strings.TrimSpace(os.Getenv("DST_PAT"))

			if cfg.Trace {
				fmt.Fprintln(os.Stderr, "[TRACE] Trace abilitato")
			}

			// Validazioni minime
			if cfg.SrcOrg == "" || cfg.SrcProject == "" {
				return fmt.Errorf("--src-org e --src-project sono obbligatori")
			}
			if cfg.SrcPAT == "" {
				return fmt.Errorf("variabile ambiente SRC_PAT mancante")
			}

			isMigration := !cfg.ListOnly && !cfg.Wizard
			if isMigration {
				if cfg.DstOrg == "" || cfg.DstProject == "" {
					return fmt.Errorf("specificare destinazione (--dst-org, --dst-project) oppure usare --list-repos/--wizard")
				}
				if cfg.DstPAT == "" {
					return fmt.Errorf("variabile ambiente DST_PAT mancante per la destinazione")
				}
			}

			// Carica elenco repo da file se fornito
			if repoListPath != "" {
				data, err := os.ReadFile(repoListPath)
				if err != nil {
					return fmt.Errorf("errore leggendo --repo-list: %w", err)
				}
				for _, ln := range strings.Split(string(data), "\n") {
					ln = strings.TrimSpace(ln)
					if ln != "" && !strings.HasPrefix(ln, "#") {
						cfg.RepoList = append(cfg.RepoList, ln)
					}
				}
			}

			// Validazione report-path
			if len(cfg.ReportFormats) > 0 {
				// Controllo formati supportati
				supported := map[string]bool{"json": true, "html": true}
				for _, f := range cfg.ReportFormats {
					if !supported[strings.ToLower(f)] {
						return fmt.Errorf("formato report non supportato: %s (sono ammessi solo json, html)", f)
					}
				}
				if cfg.ReportPath == "" {
					cfg.ReportPath = os.TempDir()
				} else {
					if info, err := os.Stat(cfg.ReportPath); err != nil || !info.IsDir() {
						return fmt.Errorf("--report-path deve essere una directory esistente: %s", cfg.ReportPath)
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

	// Definizione flag
	rootCmd.Flags().StringVar(&cfg.SrcOrg, "src-org", "", "Organizzazione sorgente (obbligatorio)")
	rootCmd.Flags().StringVar(&cfg.SrcProject, "src-project", "", "Progetto sorgente (obbligatorio)")
	rootCmd.Flags().StringVar(&cfg.DstOrg, "dst-org", "", "Organizzazione destinazione")
	rootCmd.Flags().StringVar(&cfg.DstProject, "dst-project", "", "Progetto destinazione")
	rootCmd.Flags().StringVarP(&cfg.Filter, "filter", "f", "", "Filtra repository con una regex")
	rootCmd.Flags().StringVar(&repoListPath, "repo-list", "", "File con la lista di repository da migrare (uno per riga)")
	rootCmd.Flags().BoolVar(&cfg.DryRun, "dry-run", false, "Simula l'esecuzione senza modifiche reali")
	rootCmd.Flags().BoolVar(&cfg.ForcePush, "force-push", false, "Forza il push se il repository esiste in destinazione")
	rootCmd.Flags().BoolVarP(&cfg.Trace, "trace", "t", false, "Abilita output di trace dettagliato")
	rootCmd.Flags().BoolVarP(&cfg.ListOnly, "list-repos", "l", false, "Elenca i repository sorgente e termina")
	rootCmd.Flags().BoolVarP(&cfg.Wizard, "wizard", "w", false, "Avvia la procedura guidata interattiva")
	rootCmd.Flags().BoolVarP(&cfg.ShowVersion, "version", "v", false, "Mostra la versione del programma")
	rootCmd.Flags().StringSliceVar(&cfg.ReportFormats, "report-format", []string{}, "Formati del report di migrazione (json, html), separati da virgola")
	rootCmd.Flags().StringVar(&cfg.ReportPath, "report-path", "", "Percorso directory dove salvare il report (default: directory temporanea di sistema)")

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Errore:", err)
		os.Exit(1)
	}
}
