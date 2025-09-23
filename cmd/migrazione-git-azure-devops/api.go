package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"
)

// httpClient è un'istanza condivisa di http.Client con timeout configurato
var httpClient = &http.Client{
	Timeout: 30 * time.Second,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse // non seguire i redirect
	},
}

// getRepos chiama l’API Azure DevOps per ottenere l’elenco dei repository.
// Gli errori sono restituiti al chiamante per la gestione centralizzata.
func getRepos(ctx context.Context, org, project, pat string, trace bool) ([]Repo, error) {
	path := fmt.Sprintf("_apis/git/repositories?api-version=%s", apiVersion)
	body, code, err := httpReq(ctx, "GET", org, project, path, pat, nil, trace)
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

// createRepo crea un repository in destinazione via API Azure DevOps.
// Gli errori sono restituiti al chiamante per la gestione centralizzata.
func createRepo(ctx context.Context, org, project, pat, name string, trace bool) error {
	path := fmt.Sprintf("_apis/git/repositories?api-version=%s", apiVersion)
	payload := map[string]string{"name": name}
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(payload); err != nil {
		return fmt.Errorf("errore nella codifica del payload: %w", err)
	}
	body, code, err := httpReq(ctx, "POST", org, project, path, pat, buf.Bytes(), trace)
	if err != nil {
		return err
	}
	if code != 200 && code != 201 {
		return fmt.Errorf("errore API creazione repo (HTTP %d): %s", code, string(body))
	}
	return nil
}

// httpReq effettua una richiesta HTTP autenticata in Basic (con PAT) verso Azure DevOps.
// - Non segue i redirect (CheckRedirect -> ErrUseLastResponse) così da intercettare 3xx.
// - Restituisce body, status code e l’eventuale errore di rete/IO.
func httpReq(ctx context.Context, method, org, project, path, pat string, body []byte, trace bool) ([]byte, int, error) {
	var urlStr string
	if project == "" || project == "-" {
		urlStr = fmt.Sprintf("https://dev.azure.com/%s/%s", org, path)
	} else {
		urlStr = fmt.Sprintf("https://dev.azure.com/%s/%s/%s", org, url.PathEscape(project), path)
	}
	if trace {
		fmt.Fprintln(os.Stderr, "[TRACE]", method, urlStr)
	}

	req, err := http.NewRequestWithContext(ctx, method, urlStr, bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", basicAuth(pat))
	if method == "POST" {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("errore durante la lettura della risposta: %w", err)
	}

	// Azure DevOps risponde con 302 a una pagina di login invece di 401 se il PAT non è valido.
	// Intercettiamo questo caso per fornire un errore più chiaro.
	if resp.StatusCode == http.StatusFound { // 302
		return data, http.StatusUnauthorized, fmt.Errorf("autenticazione fallita (ricevuto HTTP 302, probabile PAT non valido o scaduto)")
	}

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
		return ""
	}
	u, err := url.Parse(s)
	if err != nil {
		// Se il parsing fallisce, restituisce la stringa originale per non bloccare il logging
		return s
	}
	if u.User != nil {
		u.User = url.UserPassword("user", "***")
		return u.String()
	}
	return s
}
