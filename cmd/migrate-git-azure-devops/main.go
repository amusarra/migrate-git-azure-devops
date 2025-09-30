// Command migrate-git-azure-devops migrates Git repositories between Azure DevOps projects/organizations,
// with interactive (wizard) or non-interactive mode, dry-run support, filtering, and mirror push.
// Credentials are read from SRC_PAT and DST_PAT environment variables.
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

// Repo represents an Azure DevOps repository with main URLs.
type Repo struct {
	Name      string `json:"name"`
	RemoteURL string `json:"remoteUrl"`
	WebURL    string `json:"webUrl"`
}

// listReposResponse maps the JSON response of the repository list.
type listReposResponse struct {
	Count int    `json:"count"`
	Value []Repo `json:"value"`
}

// Config collects all CLI and environment parameters needed for migration.
type Config struct {
	SrcOrg     string
	SrcProject string
	DstOrg     string
	DstProject string
	Filter     string
	RepoList   []string
	RepoMap    map[string]string // Maps source repo names to destination repo names
	DryRun     bool
	ForcePush  bool
	Trace      bool
	Wizard     bool
	ListOnly   bool

	SrcPAT      string
	DstPAT      string
	ShowVersion bool

	ReportFormats []string // Report formats: json, html, etc.
	ReportPath    string   // Base path to save the report
}

// Summary summarizes the migration outcome for a single repository.
type Summary struct {
	Repo        string
	Action      string
	Result      string
	DstWebURL   string
	SrcWebURL   string // Source repository URL
	DstClone    string
	Skipped     bool
	ErrDetails  string
	NumBranches int      // Number of remote branches
	NumTags     int      // Number of tags
	Size        int64    // Repository size in bytes
	BranchNames []string // Remote branch names
	TagNames    []string // Tag names
}

// Report contains global report information and per-repository summaries.
type Report struct {
	StartTime   time.Time
	EndTime     time.Time
	Duration    float64 // in minutes
	Hostname    string
	Summaries   []Summary
	ProgramName string
	Version     string
	Commit      string
	BuildDate   string
}

// main is the application entry point: delegates to Execute() defined in root.go.
func main() {
	Execute()
}

// cmdListRepos lists the repositories in the source and prints them to output.
func cmdListRepos(cfg Config) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	repos, err := getRepos(ctx, cfg.SrcOrg, cfg.SrcProject, cfg.SrcPAT, cfg.Trace)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[API ERROR] Call failed for %s/%s: %v\n", cfg.SrcOrg, cfg.SrcProject, err)
		if cfg.Trace {
			fmt.Fprintf(os.Stderr, "[TRACE] Error details: %v\n", err)
		}
		os.Exit(1)
	}
	if len(repos) == 0 {
		fmt.Printf("No repository found in %s/%s\n", cfg.SrcOrg, cfg.SrcProject)
		return nil
	}
	fmt.Printf("Repositories available in %s/%s:\n\n", cfg.SrcOrg, cfg.SrcProject)
	for _, r := range repos {
		fmt.Printf("- %s\n    cloneUrl: %s\n    webUrl:   %s\n", r.Name, r.RemoteURL, r.WebURL)
	}
	return nil
}

// runWizard guides the user through an interactive procedure for selecting and migrating
// repositories, asking for confirmation before execution.
func runWizard(cfg Config) error {
	startTime := time.Now()
	hostname, _ := os.Hostname()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	in := bufio.NewReader(os.Stdin)

	// 1) List source repos
	repos, err := getRepos(ctx, cfg.SrcOrg, cfg.SrcProject, cfg.SrcPAT, cfg.Trace)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[API ERROR] Call failed for source %s/%s: %v\n", cfg.SrcOrg, cfg.SrcProject, err)
		if cfg.Trace {
			fmt.Fprintf(os.Stderr, "[TRACE] Error details: %v\n", err)
		}
		os.Exit(1)
	}
	if len(repos) == 0 {
		return fmt.Errorf("no repository found in %s/%s", cfg.SrcOrg, cfg.SrcProject)
	}
	sort.Slice(repos, func(i, j int) bool { return strings.ToLower(repos[i].Name) < strings.ToLower(repos[j].Name) })

	fmt.Printf("Repo disponibili in %s/%s:\n", cfg.SrcOrg, cfg.SrcProject)
	for i, r := range repos {
		fmt.Printf("%3d) %s\n", i+1, r.Name)
	}
	fmt.Print("\nSelect indices (e.g. 1,3-5) or press Enter to select ALL: ")
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

	// 3) Check existence in destination
	dstRepos, err := getRepos(ctx, cfg.DstOrg, cfg.DstProject, cfg.DstPAT, cfg.Trace)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[API ERROR] Call failed for destination %s/%s: %v\n", cfg.DstOrg, cfg.DstProject, err)
		if cfg.Trace {
			fmt.Fprintf(os.Stderr, "[TRACE] Error details: %v\n", err)
		}
		os.Exit(1)
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
			fmt.Print("\nSome repos already exist in destination. Perform push --force for existing ones? [y/N]: ")
			ans, _ := in.ReadString('\n')
			ans = strings.TrimSpace(strings.ToLower(ans))
			forcePush = ans == "s" || ans == "si" || ans == "y" || ans == "yes"
		}
	}

	// 4) Summary
	fmt.Println("\n===== ACTION SUMMARY =====")
	for _, r := range selected {
		action := "create+push"
		if exists[r.Name] {
			if forcePush {
				action = "push --mirror --force"
			} else {
				action = "skip (exists, no --force)"
			}
		}
		fmt.Printf("- %s: %s\n", r.Name, action)
	}
	fmt.Printf("Dry-run: %v\n", cfg.DryRun)
	fmt.Println("============================")

	// 5) Confirmation
	fmt.Print("Proceed with migration? [y/N]: ")
	confirm, _ := in.ReadString('\n')
	confirm = strings.TrimSpace(strings.ToLower(confirm))
	if confirm != "s" && confirm != "si" && confirm != "y" && confirm != "yes" {
		fmt.Println("Cancelled.")
		return nil
	}

	// 6) Execute migration with progress
	summary, err := migrateRepos(ctx, cfg, selected, exists, forcePush)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Migration error:", err)
	}

	endTime := time.Now()
	duration := endTime.Sub(startTime).Minutes()

	// 7) Final report
	printSummary(summary)
	// Generate report if requested
	if cfg.ReportFormats != nil {
		report := Report{
			StartTime:   startTime,
			EndTime:     endTime,
			Duration:    duration,
			Hostname:    hostname,
			Summaries:   summary,
			ProgramName: prog(),
			Version:     version,
			Commit:      commit,
			BuildDate:   date,
		}
		if err := generateAndSaveReport(report, cfg); err != nil {
			fmt.Fprintln(os.Stderr, "Report generation error:", err)
		}
	}
	return nil
}

// runNonInteractive performs migration without interaction, based on provided flags.
// Handles filters, lists from file, and the final summary.
func runNonInteractive(cfg Config) error {
	startTime := time.Now()
	hostname, _ := os.Hostname()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	// load source list
	srcRepos, err := getRepos(ctx, cfg.SrcOrg, cfg.SrcProject, cfg.SrcPAT, cfg.Trace)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[API ERROR] Call failed for source %s/%s: %v\n", cfg.SrcOrg, cfg.SrcProject, err)
		if cfg.Trace {
			fmt.Fprintf(os.Stderr, "[TRACE] Error details: %v\n", err)
		}
		os.Exit(1)
	}

	// build source set for fast lookup
	srcSet := map[string]Repo{}
	for _, r := range srcRepos {
		srcSet[r.Name] = r
	}

	var selected []Repo
	var preSummary []Summary

	if len(cfg.RepoList) > 0 {
		// Use exactly the names provided by the user:
		// - if they exist in source -> migrate them
		// - if NOT exist -> add an error row to the summary
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
					Result: "ERROR: source not found",
				})
			}
		}
	} else if cfg.Filter != "" {
		re, err := regexp.Compile(cfg.Filter)
		if err != nil {
			return fmt.Errorf("invalid regex: %w", err)
		}
		for _, r := range srcRepos {
			if re.MatchString(r.Name) {
				selected = append(selected, r)
			}
		}
	} else {
		selected = srcRepos
	}

	// If there are no repos to migrate but we have pre-summary errors, print the error summary and exit
	if len(selected) == 0 {
		if len(preSummary) > 0 {
			printSummary(preSummary)
			return nil
		}
		fmt.Println("No repository to migrate.")
		return nil
	}

	// destination
	dstRepos, err := getRepos(ctx, cfg.DstOrg, cfg.DstProject, cfg.DstPAT, cfg.Trace)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[API ERROR] Call failed for destination %s/%s: %v\n", cfg.DstOrg, cfg.DstProject, err)
		if cfg.Trace {
			fmt.Fprintf(os.Stderr, "[TRACE] Error details: %v\n", err)
		}
		os.Exit(1)
	}
	exists := map[string]bool{}
	for _, r := range dstRepos {
		exists[r.Name] = true
	}

	// Migrate only repos existing in source
	migSummary, err := migrateRepos(ctx, cfg, selected, exists, cfg.ForcePush)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Migration error:", err)
	}

	endTime := time.Now()
	duration := endTime.Sub(startTime).Minutes()

	// Complete summary: errors for repos not found + migration results
	all := append(preSummary, migSummary...)
	printSummary(all)
	// Generate report if requested
	if cfg.ReportFormats != nil {
		report := Report{
			StartTime:   startTime,
			EndTime:     endTime,
			Duration:    duration,
			Hostname:    hostname,
			Summaries:   all,
			ProgramName: prog(),
			Version:     version,
			Commit:      commit,
			BuildDate:   date,
		}
		if err := generateAndSaveReport(report, cfg); err != nil {
			fmt.Fprintln(os.Stderr, "Report generation error:", err)
		}
	}
	return nil
}

// migrateRepos performs migration of selected repositories:
// - clones in mirror from source into a temporary directory,
// - creates the destination repo if missing,
// - performs mirror push (with --force if requested),
// respecting dry-run and trace modes.
func migrateRepos(ctx context.Context, cfg Config, repos []Repo, dstExists map[string]bool, forcePush bool) ([]Summary, error) {
	tmpDir, err := os.MkdirTemp("", "tmp_migrazione_git_")
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			fmt.Fprintln(os.Stderr, "Error removing temporary directory:", err)
		}
	}()

	var results []Summary
	for i, r := range repos {
		// Determine destination repo name (may differ from source)
		dstRepoName := r.Name
		if cfg.RepoMap != nil {
			if mappedName, ok := cfg.RepoMap[r.Name]; ok {
				dstRepoName = mappedName
			}
		}

		if dstRepoName != r.Name {
			fmt.Printf("[%d/%d] %s -> %s\n", i+1, len(repos), r.Name, dstRepoName)
		} else {
			fmt.Printf("[%d/%d] %s\n", i+1, len(repos), r.Name)
		}
		sum := Summary{Repo: r.Name, SrcWebURL: r.WebURL}

		repoEnc := url.PathEscape(r.Name)
		dstRepoEnc := url.PathEscape(dstRepoName)
		srcProjectEnc := url.PathEscape(cfg.SrcProject)
		dstProjectEnc := url.PathEscape(cfg.DstProject)

		srcURL := fmt.Sprintf("https://%s:%s@dev.azure.com/%s/%s/_git/%s", url.QueryEscape("user"), cfg.SrcPAT, cfg.SrcOrg, srcProjectEnc, repoEnc)
		dstURL := fmt.Sprintf("https://%s:%s@dev.azure.com/%s/%s/_git/%s", url.QueryEscape("user"), cfg.DstPAT, cfg.DstOrg, dstProjectEnc, dstRepoEnc)

		dstURLRedacted := fmt.Sprintf("https://user:***@dev.azure.com/%s/%s/_git/%s", cfg.DstOrg, dstProjectEnc, dstRepoEnc)

		sum.DstClone = dstURLRedacted
		sum.DstWebURL = fmt.Sprintf("https://dev.azure.com/%s/%s/_git/%s", cfg.DstOrg, dstProjectEnc, dstRepoEnc)

		// Calculate if it already existed BEFORE migration
		origExists := dstExists[dstRepoName]

		// If it already exists and force is not wanted, skip clone and push immediately
		if origExists && !forcePush {
			if cfg.DryRun {
				fmt.Println("  [DRY] Repo already present: would skip clone and push (use --force-push to force).")
				sum.Result = "DRY-RUN"
			} else {
				fmt.Println("  Repo already present in destination. Clone/Push NOT performed (use --force-push to force).")
				sum.Result = "SKIPPED: repo already present"
			}
			results = append(results, sum)
			fmt.Println()
			continue
		}

		// Mirror clone (arrives here if: repo does not exist in dest or exists but with force-push)
		repodir := filepath.Join(tmpDir, r.Name+".git")
		if cfg.DryRun {
			sum.Action = "DRY-RUN"
			fmt.Printf("  [DRY] git clone --mirror '%s' '%s'\n", redactToken(srcURL), repodir)
		} else {
			if err := runCmd(ctx, nil, "git", "clone", "--mirror", srcURL, repodir); err != nil {
				sum.Result = "ERROR: source not found"
				sum.ErrDetails = err.Error()
				fmt.Println("  Error: source repository not found or access denied")
				results = append(results, sum)
				continue
			}
			// Get branch/tag names and count with len() to avoid double git execution
			if branchNames, err := getGitRefNames(repodir, RefTypeBranches); err == nil {
				sum.BranchNames = branchNames
				sum.NumBranches = len(branchNames)
			}
			if tagNames, err := getGitRefNames(repodir, RefTypeTags); err == nil {
				sum.TagNames = tagNames
				sum.NumTags = len(tagNames)
			}
			if size, err := dirSize(repodir); err == nil {
				sum.Size = size
			}
		}

		// Create repo in destination if missing
		if !dstExists[dstRepoName] && !cfg.DryRun {
			if err := createRepo(ctx, cfg.DstOrg, cfg.DstProject, cfg.DstPAT, dstRepoName, cfg.Trace); err != nil {
				sum.Result = "ERROR: destination creation"
				sum.ErrDetails = err.Error()
				fmt.Printf("  Error creating repo %s in destination: %v\n", dstRepoName, err)
				if cfg.Trace {
					fmt.Fprintf(os.Stderr, "[TRACE] Error details creating repo: %v\n", err)
				}
				results = append(results, sum)
				continue
			}
			dstExists[dstRepoName] = true
		} else if !dstExists[dstRepoName] && cfg.DryRun {
			fmt.Printf("  [DRY] Would create repo in destination: %s\n", dstRepoName)
		}

		// Mirror push
		if dstExists[dstRepoName] {
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
					sum.Result = "ERROR: push"
					sum.ErrDetails = err.Error()
					fmt.Println("  Error pushing to destination")
					results = append(results, sum)
					continue
				}
				fmt.Println("  OK.")
				sum.Result = "OK"
			}
		} else {
			sum.Result = "SKIPPED: missing destination"
		}

		results = append(results, sum)
		fmt.Println()
	}
	return results, nil
}
