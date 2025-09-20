package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Variabili di versione impostate da ldflags (-X main.version, etc.)
var (
	version = "dev"
	commit  = "none"
	date    = ""
)

// prog restituisce il basename dell’eseguibile in esecuzione.
func prog() string {
	return filepath.Base(os.Args[0])
}

func printVersion() {
	fmt.Printf("%s %s\ncommit: %s\nbuilt:  %s\n", prog(), version, commit, date)
}

// runCmd esegue un comando di sistema propagando l’ambiente corrente ed eventualmente
// aggiungendo variabili extra; inoltra stdout/stderr al processo chiamante.
func runCmd(ctx context.Context, env []string, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	if env != nil {
		cmd.Env = append(os.Environ(), env...)
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// generateAndSaveReport genera e salva i report nei formati specificati.
func generateAndSaveReport(report Report, cfg Config) error {
	for _, format := range cfg.ReportFormats {
		timestamp := time.Now().Format("20060102_150405")
		filename := "migration_report_" + timestamp + "." + format
		reportPath := filepath.Join(cfg.ReportPath, filename)
		fmt.Printf("Report (%s) salvato in: %s\n", format, reportPath)
		if err := generateReport(report, format, reportPath); err != nil {
			return err
		}
	}
	return nil
}

// generateReport genera il report in JSON o HTML e lo salva nel percorso specificato.
func generateReport(report Report, format, path string) error {
	switch format {
	case "json":
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return err
		}
		return os.WriteFile(path, data, 0644)
	case "html":
		html := generateHTML(report)
		return os.WriteFile(path, []byte(html), 0644)
	default:
		return fmt.Errorf("formato report non supportato: %s", format)
	}
}

// generateHTML genera una rappresentazione HTML del report come tabella.
func generateHTML(report Report) string {
	html := fmt.Sprintf(`<html><head><title>Migration Report</title></head><body>
<h1>Migration Report</h1>
<p><strong>Start Time:</strong> %s</p>
<p><strong>End Time:</strong> %s</p>
<p><strong>Duration:</strong> %.2f minutes</p>
<p><strong>Hostname:</strong> %s</p>
<table border="1">
<tr><th>Repository</th><th>Result</th><th>Source URL</th><th>Branches</th><th>Tags</th><th>Size (bytes)</th><th>Destination URL</th></tr>`,
		report.StartTime.Format("2006-01-02 15:04:05"),
		report.EndTime.Format("2006-01-02 15:04:05"),
		report.Duration,
		report.Hostname)
	for _, s := range report.Summaries {
		html += fmt.Sprintf("<tr><td>%s</td><td>%s</td><td><a href='%s'>%s</a></td><td>%d</td><td>%d</td><td>%d</td><td><a href='%s'>%s</a></td></tr>",
			s.Repo, s.Result, s.SrcWebURL, s.SrcWebURL, s.NumBranches, s.NumTags, s.Size, s.DstWebURL, s.DstWebURL)
	}
	html += "</table></body></html>"
	return html
}

// printSummary stampa una tabella di riepilogo con larghezze dinamiche per colonne,
// mostrando repository, esito e URL web di destinazione.
func printSummary(results []Summary) {
	headers := []string{"Repository", "Esito", "Azure URL"}
	// Calcola larghezze massime
	repoCol, esitoCol, azureCol := len(headers[0]), len(headers[1]), len(headers[2])
	for _, s := range results {
		if len(s.Repo) > repoCol {
			repoCol = len(s.Repo)
		}
		if len(s.Result) > esitoCol {
			esitoCol = len(s.Result)
		}
		if len(s.DstWebURL) > azureCol {
			azureCol = len(s.DstWebURL)
		}
	}
	sep := "+" + strings.Repeat("-", repoCol+2) +
		"+" + strings.Repeat("-", esitoCol+2) +
		"+" + strings.Repeat("-", azureCol+2) + "+"

	fmt.Println("===== RIEPILOGO MIGRAZIONE =====")
	fmt.Println(sep)
	fmt.Printf("| %-*s | %-*s | %-*s |\n",
		repoCol, headers[0],
		esitoCol, headers[1],
		azureCol, headers[2])
	fmt.Println(sep)
	for _, s := range results {
		fmt.Printf("| %-*s | %-*s | %-*s |\n",
			repoCol, s.Repo,
			esitoCol, s.Result,
			azureCol, s.DstWebURL)
	}
	fmt.Println(sep)
	fmt.Println(strings.Repeat("=", 32))
}

// parseElement analizza un singolo elemento (numero o intervallo) e aggiunge
// gli indici zero-based al set seen e alla slice out.
func parseElement(element string, max int, seen map[int]bool, out *[]int) error {
	element = strings.TrimSpace(element)
	if element == "" {
		return nil
	}

	if strings.Contains(element, "-") {
		// Gestione intervallo
		bits := strings.SplitN(element, "-", 2)
		if len(bits) != 2 {
			return fmt.Errorf("intervallo non valido: %s", element)
		}
		a, err1 := strconv.Atoi(strings.TrimSpace(bits[0]))
		b, err2 := strconv.Atoi(strings.TrimSpace(bits[1]))
		if err1 != nil || err2 != nil || a < 1 || b < 1 || a > b || a > max || b > max {
			return fmt.Errorf("intervallo non valido: %s", element)
		}
		for i := a; i <= b; i++ {
			if !seen[i-1] {
				*out = append(*out, i-1)
				seen[i-1] = true
			}
		}
	} else {
		// Gestione numero singolo
		n, err := strconv.Atoi(element)
		if err != nil || n < 1 || n > max {
			return fmt.Errorf("indice non valido: %s", element)
		}
		if !seen[n-1] {
			*out = append(*out, n-1)
			seen[n-1] = true
		}
	}
	return nil
}

// parseSelection converte "1,3-5" in indici zero-based ordinati univoci.
func parseSelection(sel string, max int) ([]int, error) {
	var out []int
	parts := strings.Split(sel, ",")
	seen := map[int]bool{}

	for _, p := range parts {
		if err := parseElement(p, max, seen, &out); err != nil {
			return nil, err
		}
	}

	sort.Ints(out)
	return out, nil
}

// dirSize calcola la dimensione totale di una directory in byte.
func dirSize(path string) (int64, error) {
	var size int64
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size, err
}

// countGitRefs conta il numero di riferimenti Git (es. branch o tag) in una directory repository.
func countGitRefs(repoDir, refType string) (int, error) {
	var cmd *exec.Cmd
	if refType == "branch -r" {
		// Usa ls-remote per contare branch remoti in modo più affidabile
		cmd = exec.Command("git", "ls-remote", "--heads", "origin")
	} else {
		cmd = exec.Command("git", refType)
	}
	cmd.Dir = repoDir
	output, err := cmd.Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Errore comando git %s in %s: %v\n", refType, repoDir, err)
		return 0, err
	}
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return 0, nil
	}
	return len(lines), nil
}
