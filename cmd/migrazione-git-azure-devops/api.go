package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// getRepos chiama l’API Azure DevOps per ottenere l’elenco dei repository.
// In caso di errore HTTP stampa un messaggio su stderr. Se trace è attivo,
// stampa anche il body della risposta per facilitare il debug.
func getRepos(org, project, pat string, trace bool) ([]Repo, error) {
	path := fmt.Sprintf("_apis/git/repositories?api-version=%s", apiVersion)
	body, code, err := httpReq("GET", org, project, path, pat, nil, trace)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ERRORE API] Chiamata fallita: %v\n", err)
		return nil, err
	}
	if code < 200 || code >= 300 {
		fmt.Fprintf(os.Stderr, "[ERRORE API] HTTP %d\n", code)
		if trace {
			fmt.Fprintf(os.Stderr, "[TRACE] Body: %s\n", string(body))
		}
		return nil, fmt.Errorf("errore API (HTTP %d): %s", code, string(body))
	}
	var resp listReposResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		fmt.Fprintf(os.Stderr, "[ERRORE API] Risposta non valida: %v\n", err)
		if trace {
			fmt.Fprintf(os.Stderr, "[TRACE] Body: %s\n", string(body))
		}
		return nil, fmt.Errorf("risposta non valida: %w", err)
	}
	return resp.Value, nil
}

// createRepo crea un repository in destinazione via API Azure DevOps.
// Evidenzia su stderr gli errori HTTP; in trace mostra il body di risposta.
func createRepo(org, project, pat, name string, trace bool) error {
	path := fmt.Sprintf("_apis/git/repositories?api-version=%s", apiVersion)
	payload := map[string]string{"name": name}
	b, _ := json.Marshal(payload)
	body, code, err := httpReq("POST", org, project, path, pat, b, trace)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ERRORE API] Chiamata fallita: %v\n", err)
		return err
	}
	if code != 200 && code != 201 {
		fmt.Fprintf(os.Stderr, "[ERRORE API] HTTP %d: %s\n", code, string(body))
		return fmt.Errorf("errore API creazione repo (HTTP %d): %s", code, string(body))
	}
	return nil
}

// httpReq effettua una richiesta HTTP autenticata in Basic (con PAT) verso Azure DevOps.
// - Non segue i redirect (CheckRedirect -> ErrUseLastResponse) così da intercettare 3xx.
// - Restituisce body, status code e l’eventuale errore di rete/IO.
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

	client := &http.Client{
		Timeout: 60 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse // non seguire i redirect
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			fmt.Fprintln(os.Stderr, "Errore nella chiusura della risposta HTTP:", err)
		}
	}()
	data, _ := io.ReadAll(resp.Body)
	return data, resp.StatusCode, nil
}

// basicAuth costruisce l’header Authorization Basic a partire dal PAT fornito.
func basicAuth(pat string) string {
	token := ":" + pat
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(token))
}

// redactToken oscura eventuali credenziali presenti in un URL, utile per log/trace sicuri.
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
