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

// httpClient is a shared instance of http.Client with configured timeout
var httpClient = &http.Client{
	Timeout: 30 * time.Second,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse // do not follow redirects
	},
}

// getRepos calls the Azure DevOps API to get the list of repositories.
// Errors are returned to the caller for centralized handling.
func getRepos(ctx context.Context, org, project, pat string, trace bool) ([]Repo, error) {
	path := fmt.Sprintf("_apis/git/repositories?api-version=%s", apiVersion)
	body, code, err := httpReq(ctx, "GET", org, project, path, pat, nil, trace)
	if err != nil {
		return nil, err
	}
	if code < 200 || code >= 300 {
		return nil, fmt.Errorf("API error (HTTP %d): %s", code, string(body))
	}
	var resp listReposResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("invalid response: %w", err)
	}
	return resp.Value, nil
}

// createRepo creates a destination repository via Azure DevOps API.
// Errors are returned to the caller for centralized handling.
func createRepo(ctx context.Context, org, project, pat, name string, trace bool) error {
	path := fmt.Sprintf("_apis/git/repositories?api-version=%s", apiVersion)
	payload := map[string]string{"name": name}
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(payload); err != nil {
		return fmt.Errorf("error encoding payload: %w", err)
	}
	body, code, err := httpReq(ctx, "POST", org, project, path, pat, buf.Bytes(), trace)
	if err != nil {
		return err
	}
	if code != 200 && code != 201 {
		return fmt.Errorf("API error creating repo (HTTP %d): %s", code, string(body))
	}
	return nil
}

// httpReq performs an authenticated HTTP request using Basic (with PAT) to Azure DevOps.
// - Does not follow redirects (CheckRedirect -> ErrUseLastResponse) to intercept 3xx.
// - Returns body, status code, and any network/IO error.
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
	defer func() {
		if err := resp.Body.Close(); err != nil {
			fmt.Fprintln(os.Stderr, "Error closing HTTP response:", err)
		}
	}()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("error reading response: %w", err)
	}

	// Azure DevOps responds with 302 to a login page instead of 401 if the PAT is invalid.
	// We intercept this case to provide a clearer error.
	if resp.StatusCode == http.StatusFound { // 302
		return data, http.StatusUnauthorized, fmt.Errorf("authentication failed (received HTTP 302, likely invalid or expired PAT)")
	}

	return data, resp.StatusCode, nil
}

// basicAuth builds the Authorization Basic header from the provided PAT.
func basicAuth(pat string) string {
	token := ":" + pat
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(token))
}

// redactToken masks any credentials present in a URL, useful for safe log/trace.
func redactToken(s string) string {
	if s == "" {
		return ""
	}
	u, err := url.Parse(s)
	if err != nil {
		// If parsing fails, return the original string to not block logging
		return s
	}
	if u.User != nil {
		u.User = url.UserPassword("user", "***")
		return u.String()
	}
	return s
}
