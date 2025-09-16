package main

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	apiVersion = "7.1"
)

type Repo struct {
	Name      string `json:"name"`
	RemoteURL string `json:"remoteUrl"`
	WebURL    string `json:"webUrl"`
}

type listReposResponse struct {
	Count int    `json:"count"`
	Value []Repo `json:"value"`
}

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

	SrcPAT string
	DstPAT string
}

type Summary struct {
	Repo       string
	Action     string
	Result     string
	DstWebURL  string
	DstClone   string
	Skipped    bool
	ErrDetails string
}

func main() {
	cfg, err := parseArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "Errore:", err)
		os.Exit(2)
	}
	if cfg.Trace {
		fmt.Fprintln(os.Stderr, "[TRACE] Trace abilitato")
	}

	// Validazioni minime
	if cfg.SrcOrg == "" || cfg.SrcProject == "" {
		fmt.Fprintln(os.Stderr, "Errore: --src-org e --src-project sono obbligatori")
		os.Exit(2)
	}
	if cfg.ListOnly || cfg.Wizard || (cfg.DstOrg != "" && cfg.DstProject != "") {
		// ok
	} else {
		fmt.Fprintln(os.Stderr, "Errore: specificare destinazione (--dst-org, --dst-project) oppure usare --list-repos/--wizard")
		os.Exit(2)
	}

	// Modalità: lista repository e termina
	if cfg.ListOnly {
		if err := cmdListRepos(cfg); err != nil {
			os.Exit(1)
		}
		return
	}

	// Wizard interattivo
	if cfg.Wizard {
		if err := runWizard(cfg); err != nil {
			fmt.Fprintln(os.Stderr, "Errore:", err)
			os.Exit(1)
		}
		return
	}

	// Modalità non interattiva (flag)
	if err := runNonInteractive(cfg); err != nil {
		fmt.Fprintln(os.Stderr, "Errore:", err)
		os.Exit(1)
	}
}

func parseArgs(args []string) (Config, error) {
	cfg := Config{}
	// Env PAT
	cfg.SrcPAT = strings.TrimSpace(os.Getenv("SRC_PAT"))
	cfg.DstPAT = strings.TrimSpace(os.Getenv("DST_PAT"))

	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--src-org", "-so":
			i++
			cfg.SrcOrg = val(args, i)
		case "--src-project", "-sp":
			i++
			cfg.SrcProject = val(args, i)
		case "--dst-org", "-do":
			i++
			cfg.DstOrg = val(args, i)
		case "--dst-project", "-dp":
			i++
			cfg.DstProject = val(args, i)
		case "--filter", "-f":
			i++
			cfg.Filter = val(args, i)
		case "--repo-list", "-rl":
			i++
			path := val(args, i)
			if path != "" {
				lines, err := os.ReadFile(path)
				if err != nil {
					return cfg, err
				}
				for _, ln := range strings.Split(string(lines), "\n") {
					ln = strings.TrimSpace(ln)
					if ln != "" && !strings.HasPrefix(ln, "#") {
						cfg.RepoList = append(cfg.RepoList, ln)
					}
				}
			}
		case "--dry-run":
			cfg.DryRun = true
		case "--force-push", "-fp":
			cfg.ForcePush = true
		case "--trace", "-t":
			cfg.Trace = true
		case "--list-repos":
			cfg.ListOnly = true
		case "--wizard":
			cfg.Wizard = true
		case "-h", "--help":
			usage()
			os.Exit(0)
		default:
			return cfg, fmt.Errorf("argomento sconosciuto: %s", a)
		}
	}

	// PAT richiesti: sempre per list/wizard e per migrazione
	if cfg.SrcPAT == "" {
		return cfg, errors.New("variabile ambiente SRC_PAT mancante")
	}
	if !cfg.ListOnly && cfg.DstOrg != "" && cfg.DstProject != "" && cfg.DstPAT == "" {
		return cfg, errors.New("variabile ambiente DST_PAT mancante per la destinazione")
	}

	return cfg, nil
}

func val(args []string, i int) string {
	if i >= 0 && i < len(args) {
		return args[i]
	}
	return ""
}

// Restituisce il nome del programma (basename dell'eseguibile)
func prog() string {
	return filepath.Base(os.Args[0])
}

func usage() {
	name := prog()
	fmt.Printf(`Uso:
  %s --src-org ORG --src-project PROJ [--dst-org ORG --dst-project PROJ]
                 [--filter REGEX] [--repo-list FILE] [--dry-run] [--force-push]
                 [--trace] [--list-repos] [--wizard]

Esempi:
  %s --src-org myorg --src-project MyProject --list-repos
  %s -so src -sp Proj -do dst -dp ProjDst --wizard
  %s -so src -sp Proj -do dst -dp ProjDst -f '^ansc-.*$' --dry-run
`, name, name, name, name)
}

func cmdListRepos(cfg Config) error {
	repos, err := getRepos(cfg.SrcOrg, cfg.SrcProject, cfg.SrcPAT, cfg.Trace)
	if err != nil {
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

func runWizard(cfg Config) error {
	in := bufio.NewReader(os.Stdin)

	// 1) Lista repo sorgente
	repos, err := getRepos(cfg.SrcOrg, cfg.SrcProject, cfg.SrcPAT, cfg.Trace)
	if err != nil {
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
	dstRepos, err := getRepos(cfg.DstOrg, cfg.DstProject, cfg.DstPAT, cfg.Trace)
	if err != nil {
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
	if !(confirm == "s" || confirm == "si" || confirm == "y" || confirm == "yes") {
		fmt.Println("Annullato.")
		return nil
	}

	// 6) Esegui migrazione con avanzamento
	summary, err := migrateRepos(cfg, selected, exists, forcePush)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Errore migrazione:", err)
	}

	// 7) Report finale
	printSummary(summary)
	return nil
}

func runNonInteractive(cfg Config) error {
	// carica lista sorgente
	srcRepos, err := getRepos(cfg.SrcOrg, cfg.SrcProject, cfg.SrcPAT, cfg.Trace)
	if err != nil {
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
	dstRepos, err := getRepos(cfg.DstOrg, cfg.DstProject, cfg.DstPAT, cfg.Trace)
	if err != nil {
		return err
	}
	exists := map[string]bool{}
	for _, r := range dstRepos {
		exists[r.Name] = true
	}

	// Migrazione dei soli repo esistenti nella sorgente
	migSummary, err := migrateRepos(cfg, selected, exists, cfg.ForcePush)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Errore migrazione:", err)
	}

	// Riepilogo completo: errori per repo non trovati + risultati migrazione
	all := append(preSummary, migSummary...)
	printSummary(all)
	return nil
}

func migrateRepos(cfg Config, repos []Repo, dstExists map[string]bool, forcePush bool) ([]Summary, error) {
	tmpDir, err := os.MkdirTemp("", "tmp_migrazione_git_")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	var results []Summary
	for i, r := range repos {
		fmt.Printf("[%d/%d] %s\n", i+1, len(repos), r.Name)
		sum := Summary{Repo: r.Name}

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
			if err := runCmd(nil, "git", "clone", "--mirror", srcURL, repodir); err != nil {
				sum.Result = "ERRORE: sorgente non trovato"
				sum.ErrDetails = err.Error()
				fmt.Println("  Errore: repository sorgente non trovato o accesso negato")
				results = append(results, sum)
				continue
			}
		}

		// Crea repo in destinazione se mancante
		if !dstExists[r.Name] && !cfg.DryRun {
			if err := createRepo(cfg.DstOrg, cfg.DstProject, cfg.DstPAT, r.Name, cfg.Trace); err != nil {
				sum.Result = "ERRORE: creazione destinazione"
				sum.ErrDetails = err.Error()
				fmt.Println("  Errore nella creazione della repo in destinazione")
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
				if err := runCmd(nil, "git", args...); err != nil {
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

func printSummary(results []Summary) {
	fmt.Println("===== RIEPILOGO MIGRAZIONE =====")
	fmt.Printf("%-40s | %-22s | %-50s | %s\n", "Repository", "Esito", "Azure URL", "URL di clone")
	fmt.Printf("%-40s-+-%-22s-+-%-50s-+-%s\n", strings.Repeat("-", 40), strings.Repeat("-", 22), strings.Repeat("-", 50), strings.Repeat("-", 45))
	for _, s := range results {
		fmt.Printf("%-40s | %-22s | %-50s | %s\n", s.Repo, s.Result, s.DstWebURL, s.DstClone)
	}
	fmt.Println(strings.Repeat("=", 32))
}

func runCmd(env []string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	if env != nil {
		cmd.Env = append(os.Environ(), env...)
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// --- API helpers ---

func getRepos(org, project, pat string, trace bool) ([]Repo, error) {
	path := fmt.Sprintf("_apis/git/repositories?api-version=%s", apiVersion)
	body, code, err := httpReq("GET", org, project, path, pat, nil, trace)
	if err != nil {
		return nil, err
	}
	if code < 200 || code >= 300 {
		return nil, fmt.Errorf("errore API (HTTP %d): %s", code, string(body))
	}
	var resp listReposResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("risposta non valida: %w", err)
	}
	return resp.Value, nil
}

func createRepo(org, project, pat, name string, trace bool) error {
	path := fmt.Sprintf("_apis/git/repositories?api-version=%s", apiVersion)
	payload := map[string]string{"name": name}
	b, _ := json.Marshal(payload)
	body, code, err := httpReq("POST", org, project, path, pat, b, trace)
	if err != nil {
		return err
	}
	if code != 200 && code != 201 {
		return fmt.Errorf("errore API creazione repo (HTTP %d): %s", code, string(body))
	}
	return nil
}

func httpReq(method, org, project, path, pat string, body []byte, trace bool) ([]byte, int, error) {
	var urlStr string
	if project == "" || project == "-" {
		urlStr = fmt.Sprintf("https://dev.azure.com/%s/%s", org, path)
	} else {
		urlStr = fmt.Sprintf("https://dev.azure.com/%s/%s/%s", org, url.PathEscape(project), path)
	}
	if trace {
		fmt.Fprintln(os.Stderr, "[TRACE]", method, urlStr)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, method, urlStr, strings.NewReader(string(body)))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", basicAuth(pat))
	if method == "POST" {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body) // usa direttamente io.ReadAll
	return data, resp.StatusCode, nil
}

func basicAuth(pat string) string {
	token := ":" + pat
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(token))
}

func redactToken(s string) string {
	if s == "" {
		return s
	}
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
		if j := strings.Index(s, "@"); j > 0 {
			if k := strings.Index(s, ":"); k > 0 && k < j {
				s = "https://user:***@" + s[j+1:]
				return s
			}
		}
	}
	return s
}

// --- utils ---

func parseSelection(sel string, max int) ([]int, error) {
	var out []int
	parts := strings.Split(sel, ",")
	seen := map[int]bool{}
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if strings.Contains(p, "-") {
			bits := strings.SplitN(p, "-", 2)
			if len(bits) != 2 {
				return nil, fmt.Errorf("intervallo non valido: %s", p)
			}
			a, err1 := strconv.Atoi(strings.TrimSpace(bits[0]))
			b, err2 := strconv.Atoi(strings.TrimSpace(bits[1]))
			if err1 != nil || err2 != nil || a < 1 || b < 1 || a > b || a > max || b > max {
				return nil, fmt.Errorf("intervallo non valido: %s", p)
			}
			for i := a; i <= b; i++ {
				if !seen[i-1] {
					out = append(out, i-1)
					seen[i-1] = true
				}
			}
		} else {
			n, err := strconv.Atoi(p)
			if err != nil || n < 1 || n > max {
				return nil, fmt.Errorf("indice non valido: %s", p)
			}
			if !seen[n-1] {
				out = append(out, n-1)
				seen[n-1] = true
			}
		}
	}
	sort.Ints(out)
	return out, nil
}
