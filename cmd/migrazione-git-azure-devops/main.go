// Command migrazione-git-azure-devops migra repository Git tra progetti/organizzazioni Azure DevOps,
// con modalità interattiva (wizard) o non interattiva, supporto dry-run, filtraggio e push mirror.
// Le credenziali sono lette dalle variabili SRC_PAT e DST_PAT.
package main

import (
	"bufio"
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	apiVersion = "7.1"
)

// Repo rappresenta un repository Azure DevOps con i principali URL.
type Repo struct {
	Name      string `json:"name"`
	RemoteURL string `json:"remoteUrl"`
	WebURL    string `json:"webUrl"`
}

// listReposResponse mappa la risposta JSON della lista repository.
type listReposResponse struct {
	Count int    `json:"count"`
	Value []Repo `json:"value"`
}

// Config raccoglie tutti i parametri CLI e di ambiente necessari alla migrazione.
type Config struct {
	SrcOrg     string
	SrcProject string
	DstOrg     string
	DstProject string
	Filter     string
	RepoList   []string
	DryRun     bool
	ForcePush  bool
	Trace      bool
	Wizard     bool
	ListOnly   bool

	SrcPAT      string
	DstPAT      string
	ShowVersion bool

	ReportFormats []string // Formati del report: json, html, etc.
	ReportPath    string   // Percorso base per salvare il report
}

// Summary riassume l’esito della migrazione per un singolo repository.
type Summary struct {
	Repo        string
	Action      string
	Result      string
	DstWebURL   string
	SrcWebURL   string // URL del repository sorgente
	DstClone    string
	Skipped     bool
	ErrDetails  string
	NumBranches int      // Numero di branch remoti
	NumTags     int      // Numero di tag
	Size        int64    // Dimensione del repository in byte
	BranchNames []string // Nomi dei branch remoti
	TagNames    []string // Nomi dei tag
}

// Report contiene le informazioni globali del report e i riepiloghi per repository.
type Report struct {
	StartTime time.Time
	EndTime   time.Time
	Duration  float64 // in minuti
	Hostname  string
	Summaries []Summary
}

// main è il punto di ingresso dell’applicazione: delega a Execute() definita in root.go.
func main() {
	Execute()
}

// cmdListRepos elenca i repository nella sorgente e li stampa in output.
func cmdListRepos(cfg Config) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	repos, err := getRepos(ctx, cfg.SrcOrg, cfg.SrcProject, cfg.SrcPAT, cfg.Trace)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ERRORE API] Chiamata fallita per %s/%s: %v\n", cfg.SrcOrg, cfg.SrcProject, err)
		if cfg.Trace {
			fmt.Fprintf(os.Stderr, "[TRACE] Dettagli errore: %v\n", err)
		}
		return err
	}
	if len(repos) == 0 {
		fmt.Printf("Nessun repository trovato in %s/%s\n", cfg.SrcOrg, cfg.SrcProject)
		return nil
	}
	fmt.Printf("Repository disponibili in %s/%s:\n\n", cfg.SrcOrg, cfg.SrcProject)
	for _, r := range repos {
		fmt.Printf("- %s\n    cloneUrl: %s\n    webUrl:   %s\n", r.Name, r.RemoteURL, r.WebURL)
	}
	return nil
}

// runWizard guida l’utente in una procedura interattiva di selezione e migrazione
// dei repository, chiedendo conferma prima dell’esecuzione.
func runWizard(cfg Config) error {
	startTime := time.Now()
	hostname, _ := os.Hostname()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	in := bufio.NewReader(os.Stdin)

	// 1) Lista repo sorgente
	repos, err := getRepos(ctx, cfg.SrcOrg, cfg.SrcProject, cfg.SrcPAT, cfg.Trace)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ERRORE API] Chiamata fallita per sorgente %s/%s: %v\n", cfg.SrcOrg, cfg.SrcProject, err)
		if cfg.Trace {
			fmt.Fprintf(os.Stderr, "[TRACE] Dettagli errore: %v\n", err)
		}
		return err
	}
	if len(repos) == 0 {
		return fmt.Errorf("nessun repository trovato in %s/%s", cfg.SrcOrg, cfg.SrcProject)
	}
	sort.Slice(repos, func(i, j int) bool { return strings.ToLower(repos[i].Name) < strings.ToLower(repos[j].Name) })

	fmt.Printf("Repo disponibili in %s/%s:\n", cfg.SrcOrg, cfg.SrcProject)
	for i, r := range repos {
		fmt.Printf("%3d) %s\n", i+1, r.Name)
	}
	fmt.Print("\nSeleziona indici (es. 1,3-5) oppure premi Invio per selezionare TUTTI: ")
	selection, _ := in.ReadString('\n')
	selection = strings.TrimSpace(selection)

	var selected []Repo
	if selection == "" {
		selected = repos
	} else {
		idx, err := parseSelection(selection, len(repos))
		if err != nil {
			return err
		}
		for _, i := range idx {
			selected = append(selected, repos[i])
		}
	}

	// 3) Verifica esistenza in destinazione
	dstRepos, err := getRepos(ctx, cfg.DstOrg, cfg.DstProject, cfg.DstPAT, cfg.Trace)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ERRORE API] Chiamata fallita per destinazione %s/%s: %v\n", cfg.DstOrg, cfg.DstProject, err)
		if cfg.Trace {
			fmt.Fprintf(os.Stderr, "[TRACE] Dettagli errore: %v\n", err)
		}
		return err
	}
	exists := map[string]bool{}
	for _, r := range dstRepos {
		exists[r.Name] = true
	}

	// Force push?
	forcePush := cfg.ForcePush
	if !forcePush {
		anyExists := false
		for _, r := range selected {
			if exists[r.Name] {
				anyExists = true
				break
			}
		}
		if anyExists {
			fmt.Print("\nAlcuni repo esistono già in destinazione. Eseguire push --force per quelli esistenti? [s/N]: ")
			ans, _ := in.ReadString('\n')
			ans = strings.TrimSpace(strings.ToLower(ans))
			forcePush = ans == "s" || ans == "si" || ans == "y" || ans == "yes"
		}
	}

	// 4) Riepilogo
	fmt.Println("\n===== RIEPILOGO AZIONI =====")
	for _, r := range selected {
		action := "create+push"
		if exists[r.Name] {
			if forcePush {
				action = "push --mirror --force"
			} else {
				action = "skip (esiste, no --force)"
			}
		}
		fmt.Printf("- %s: %s\n", r.Name, action)
	}
	fmt.Printf("Dry-run: %v\n", cfg.DryRun)
	fmt.Println("============================")

	// 5) Conferma
	fmt.Print("Procedere con la migrazione? [s/N]: ")
	confirm, _ := in.ReadString('\n')
	confirm = strings.TrimSpace(strings.ToLower(confirm))
	if confirm != "s" && confirm != "si" && confirm != "y" && confirm != "yes" {
		fmt.Println("Annullato.")
		return nil
	}

	// 6) Esegui migrazione con avanzamento
	summary, err := migrateRepos(ctx, cfg, selected, exists, forcePush)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Errore migrazione:", err)
	}

	endTime := time.Now()
	duration := endTime.Sub(startTime).Minutes()

	// 7) Report finale
	printSummary(summary)
	// Genera report se richiesto
	if cfg.ReportFormats != nil {
		report := Report{
			StartTime: startTime,
			EndTime:   endTime,
			Duration:  duration,
			Hostname:  hostname,
			Summaries: summary,
		}
		if err := generateAndSaveReport(report, cfg); err != nil {
			fmt.Fprintln(os.Stderr, "Errore generazione report:", err)
		}
	}
	return nil
}

// runNonInteractive esegue la migrazione senza interazione, in base ai flag forniti.
// Gestisce filtri, liste da file e il riepilogo finale.
func runNonInteractive(cfg Config) error {
	startTime := time.Now()
	hostname, _ := os.Hostname()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	// carica lista sorgente
	srcRepos, err := getRepos(ctx, cfg.SrcOrg, cfg.SrcProject, cfg.SrcPAT, cfg.Trace)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ERRORE API] Chiamata fallita per sorgente %s/%s: %v\n", cfg.SrcOrg, cfg.SrcProject, err)
		if cfg.Trace {
			fmt.Fprintf(os.Stderr, "[TRACE] Dettagli errore: %v\n", err)
		}
		return err
	}

	// costruisci set sorgente per lookup rapido
	srcSet := map[string]Repo{}
	for _, r := range srcRepos {
		srcSet[r.Name] = r
	}

	var selected []Repo
	var preSummary []Summary

	if len(cfg.RepoList) > 0 {
		// Usa esattamente i nomi forniti dall'utente:
		// - se esistono in sorgente -> li migriamo
		// - se NON esistono -> aggiungiamo una riga di errore al riepilogo
		for _, name := range cfg.RepoList {
			nm := strings.TrimSpace(name)
			if nm == "" {
				continue
			}
			if r, ok := srcSet[nm]; ok {
				selected = append(selected, r)
			} else {
				preSummary = append(preSummary, Summary{
					Repo:   nm,
					Result: "ERRORE: sorgente non trovato",
				})
			}
		}
	} else if cfg.Filter != "" {
		re, err := regexp.Compile(cfg.Filter)
		if err != nil {
			return fmt.Errorf("regex non valida: %w", err)
		}
		for _, r := range srcRepos {
			if re.MatchString(r.Name) {
				selected = append(selected, r)
			}
		}
	} else {
		selected = srcRepos
	}

	// Se non ci sono repo da migrare ma abbiamo errori pre-summary, stampa il riepilogo errori ed esci
	if len(selected) == 0 {
		if len(preSummary) > 0 {
			printSummary(preSummary)
			return nil
		}
		fmt.Println("Nessun repository da migrare.")
		return nil
	}

	// destinazione
	dstRepos, err := getRepos(ctx, cfg.DstOrg, cfg.DstProject, cfg.DstPAT, cfg.Trace)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ERRORE API] Chiamata fallita per destinazione %s/%s: %v\n", cfg.DstOrg, cfg.DstProject, err)
		if cfg.Trace {
			fmt.Fprintf(os.Stderr, "[TRACE] Dettagli errore: %v\n", err)
		}
		return err
	}
	exists := map[string]bool{}
	for _, r := range dstRepos {
		exists[r.Name] = true
	}

	// Migrazione dei soli repo esistenti nella sorgente
	migSummary, err := migrateRepos(ctx, cfg, selected, exists, cfg.ForcePush)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Errore migrazione:", err)
	}

	endTime := time.Now()
	duration := endTime.Sub(startTime).Minutes()

	// Riepilogo completo: errori per repo non trovati + risultati migrazione
	all := append(preSummary, migSummary...)
	printSummary(all)
	// Genera report se richiesto
	if cfg.ReportFormats != nil {
		report := Report{
			StartTime: startTime,
			EndTime:   endTime,
			Duration:  duration,
			Hostname:  hostname,
			Summaries: all,
		}
		if err := generateAndSaveReport(report, cfg); err != nil {
			fmt.Fprintln(os.Stderr, "Errore generazione report:", err)
		}
	}
	return nil
}

// migrateRepos esegue la migrazione dei repository selezionati:
// - clona in mirror dal sorgente in una directory temporanea,
// - crea la repo di destinazione se mancante,
// - esegue il push mirror (con --force se richiesto),
// rispettando le modalità dry-run e trace.
func migrateRepos(ctx context.Context, cfg Config, repos []Repo, dstExists map[string]bool, forcePush bool) ([]Summary, error) {
	tmpDir, err := os.MkdirTemp("", "tmp_migrazione_git_")
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			fmt.Fprintln(os.Stderr, "Errore nella rimozione della directory temporanea:", err)
		}
	}()

	var results []Summary
	for i, r := range repos {
		fmt.Printf("[%d/%d] %s\n", i+1, len(repos), r.Name)
		sum := Summary{Repo: r.Name, SrcWebURL: r.WebURL}

		repoEnc := url.PathEscape(r.Name)
		srcProjectEnc := url.PathEscape(cfg.SrcProject)
		dstProjectEnc := url.PathEscape(cfg.DstProject)

		srcURL := fmt.Sprintf("https://%s:%s@dev.azure.com/%s/%s/_git/%s", url.QueryEscape("user"), cfg.SrcPAT, cfg.SrcOrg, srcProjectEnc, repoEnc)
		dstURL := fmt.Sprintf("https://%s:%s@dev.azure.com/%s/%s/_git/%s", url.QueryEscape("user"), cfg.DstPAT, cfg.DstOrg, dstProjectEnc, repoEnc)

		dstURLRedacted := fmt.Sprintf("https://user:***@dev.azure.com/%s/%s/_git/%s", cfg.DstOrg, dstProjectEnc, repoEnc)

		sum.DstClone = dstURLRedacted
		sum.DstWebURL = fmt.Sprintf("https://dev.azure.com/%s/%s/_git/%s", cfg.DstOrg, dstProjectEnc, repoEnc)

		// Calcola se esisteva già PRIMA della migrazione
		origExists := dstExists[r.Name]

		// Se esiste già e non si vuole forzare, salta subito clone e push
		if origExists && !forcePush {
			if cfg.DryRun {
				fmt.Println("  [DRY] Repo già presente: salterei clone e push (usa --force-push per forzare).")
				sum.Result = "DRY-RUN"
			} else {
				fmt.Println("  Repo già presente in destinazione. Clone/Push NON eseguiti (usa --force-push per forzare).")
				sum.Result = "SKIPPED: repo già presente"
			}
			results = append(results, sum)
			fmt.Println()
			continue
		}

		// Clone mirror (si arriva qui se: repo non esiste in dest oppure esiste ma con force-push)
		repodir := filepath.Join(tmpDir, r.Name+".git")
		if cfg.DryRun {
			sum.Action = "DRY-RUN"
			fmt.Printf("  [DRY] git clone --mirror '%s' '%s'\n", redactToken(srcURL), repodir)
		} else {
			if err := runCmd(ctx, nil, "git", "clone", "--mirror", srcURL, repodir); err != nil {
				sum.Result = "ERRORE: sorgente non trovato"
				sum.ErrDetails = err.Error()
				fmt.Println("  Errore: repository sorgente non trovato o accesso negato")
				results = append(results, sum)
				continue
			}
			// Calcola numero di branch, tag, nomi e dimensione dopo il clone
			if numBranches, err := countGitRefs(repodir, RefTypeBranches); err == nil {
				sum.NumBranches = numBranches
			}
			if branchNames, err := getGitRefNames(repodir, RefTypeBranches); err == nil {
				sum.BranchNames = branchNames
			}
			if numTags, err := countGitRefs(repodir, RefTypeTags); err == nil {
				sum.NumTags = numTags
			}
			if tagNames, err := getGitRefNames(repodir, RefTypeTags); err == nil {
				sum.TagNames = tagNames
			}
			if size, err := dirSize(repodir); err == nil {
				sum.Size = size
			}
		}

		// Crea repo in destinazione se mancante
		if !dstExists[r.Name] && !cfg.DryRun {
			if err := createRepo(ctx, cfg.DstOrg, cfg.DstProject, cfg.DstPAT, r.Name, cfg.Trace); err != nil {
				sum.Result = "ERRORE: creazione destinazione"
				sum.ErrDetails = err.Error()
				fmt.Printf("  Errore nella creazione della repo %s in destinazione: %v\n", r.Name, err)
				if cfg.Trace {
					fmt.Fprintf(os.Stderr, "[TRACE] Dettagli errore creazione repo: %v\n", err)
				}
				results = append(results, sum)
				continue
			}
			dstExists[r.Name] = true
		} else if !dstExists[r.Name] && cfg.DryRun {
			fmt.Printf("  [DRY] Creerei la repo in destinazione: %s\n", r.Name)
		}

		// Push mirror
		if dstExists[r.Name] {
			if cfg.DryRun {
				if origExists && forcePush {
					fmt.Printf("  [DRY] (cd '%s' && git push --mirror --force '%s')\n", repodir, dstURLRedacted)
				} else {
					fmt.Printf("  [DRY] (cd '%s' && git push --mirror '%s')\n", repodir, dstURLRedacted)
				}
				sum.Result = "DRY-RUN"
			} else {
				args := []string{"-C", repodir, "push", "--mirror"}
				if origExists && forcePush {
					args = append(args, "--force")
				}
				args = append(args, dstURL)
				if err := runCmd(ctx, nil, "git", args...); err != nil {
					sum.Result = "ERRORE: push"
					sum.ErrDetails = err.Error()
					fmt.Println("  Errore nel push verso destinazione")
					results = append(results, sum)
					continue
				}
				fmt.Println("  OK.")
				sum.Result = "OK"
			}
		} else {
			sum.Result = "SKIPPED: destinazione mancante"
		}

		results = append(results, sum)
		fmt.Println()
	}
	return results, nil
}
