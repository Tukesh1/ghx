package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// UserAgent identifies ghx to the GitHub API; main sets it to "ghx/<version>".
var UserAgent = "ghx"

// retryDelay is the base backoff between retries of a failed read (attempt² × retryDelay).
var retryDelay = 700 * time.Millisecond

// Client talks to the GitHub GraphQL and REST APIs with a bearer token.
type Client struct {
	token      string
	http       *http.Client
	restURL    string
	graphqlURL string
}

// NewClient returns a Client for api.github.com with a sane request timeout.
func NewClient(token string) *Client {
	return &Client{
		token:      token,
		http:       &http.Client{Timeout: 25 * time.Second},
		restURL:    "https://api.github.com",
		graphqlURL: "https://api.github.com/graphql",
	}
}

// NewTestClient points a Client at a fake API server (an httptest.Server URL).
func NewTestClient(token, baseURL string) *Client {
	c := NewClient(token)
	c.restURL, c.graphqlURL = baseURL, baseURL+"/graphql"
	return c
}

// ErrStatsPending is returned when GitHub answers 202: repository statistics are still being computed.
var ErrStatsPending = errors.New("github is computing repository statistics")

// APIError is a non-2xx answer from GitHub, with a human-readable explanation.
type APIError struct {
	Status  int
	Path    string
	Message string
	Reset   time.Time // set when the rate limit is exhausted
}

func (e *APIError) Error() string {
	switch {
	case e.Status == http.StatusUnauthorized:
		return "GitHub rejected the token (401) · run `gh auth refresh` or set a valid GITHUB_TOKEN"
	case !e.Reset.IsZero():
		return "GitHub API rate limit exceeded · resets at " + e.Reset.Local().Format("15:04")
	case e.Status >= 500:
		return fmt.Sprintf("GitHub is having trouble (HTTP %d) · try again shortly", e.Status)
	case e.Message != "":
		return fmt.Sprintf("%s (HTTP %d)", e.Message, e.Status)
	default:
		return fmt.Sprintf("%s: HTTP %d", e.Path, e.Status)
	}
}

// IsNotFound reports whether err is a 404 or a 403 that hides a resource (both mean "not available to you").
func IsNotFound(err error) bool {
	var ae *APIError
	return errors.As(err, &ae) && (ae.Status == http.StatusNotFound || ae.Status == http.StatusForbidden && ae.Reset.IsZero())
}

// retryable reports whether a failed read is worth repeating: network errors and 5xx answers.
func retryable(ctx context.Context, err error) bool {
	if ctx.Err() != nil {
		return false
	}
	var ae *APIError
	if errors.As(err, &ae) {
		return ae.Status >= 500
	}
	var ne net.Error
	return errors.As(err, &ne) || errors.Is(err, io.ErrUnexpectedEOF)
}

// do sends one request. body (if non-nil) is JSON-encoded; out receives the decoded JSON answer,
// or the raw text when it is a *string. Reads (and GraphQL queries) are retried on 5xx/network errors.
func (c *Client) do(ctx context.Context, method, url string, body, out any) error {
	var payload []byte
	if body != nil {
		var err error
		if payload, err = json.Marshal(body); err != nil {
			return err
		}
	}
	idempotent := method == http.MethodGet || url == c.graphqlURL
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-time.After(time.Duration(attempt*attempt) * retryDelay):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		err = c.once(ctx, method, url, payload, out)
		if err == nil || !idempotent || !retryable(ctx, err) {
			return err
		}
	}
	return err
}

func (c *Client) once(ctx context.Context, method, url string, payload []byte, out any) error {
	var rd io.Reader
	if payload != nil {
		rd = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", UserAgent)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusAccepted && method == http.MethodGet {
		return ErrStatsPending
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return apiError(resp, strings.TrimPrefix(strings.TrimPrefix(url, c.restURL), c.graphqlURL))
	}
	if out == nil || resp.StatusCode == http.StatusNoContent {
		return nil
	}
	if s, ok := out.(*string); ok {
		b, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		*s = string(b)
		return err
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode %s: %w", req.URL.Path, err)
	}
	return nil
}

func apiError(resp *http.Response, path string) error {
	e := &APIError{Status: resp.StatusCode, Path: path}
	var msg struct {
		Message string `json:"message"`
	}
	_ = json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&msg)
	e.Message = msg.Message
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
		if resp.Header.Get("X-RateLimit-Remaining") == "0" {
			if sec, err := strconv.ParseInt(resp.Header.Get("X-RateLimit-Reset"), 10, 64); err == nil {
				e.Reset = time.Unix(sec, 0)
			}
		}
		if e.Reset.IsZero() && strings.Contains(strings.ToLower(e.Message), "rate limit") {
			e.Reset = time.Now().Add(time.Minute)
		}
	}
	return e
}

// rest performs a REST call against a path such as "/repos/o/r/pulls".
func (c *Client) rest(ctx context.Context, method, path string, body, out any) error {
	return c.do(ctx, method, c.restURL+path, body, out)
}

// get is a REST GET.
func (c *Client) get(ctx context.Context, path string, out any) error {
	return c.rest(ctx, http.MethodGet, path, nil, out)
}

type gqlError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
}

// graphQL runs a query and decodes its data into out. Partial answers (data plus errors, e.g. a
// SAML-protected org hiding some nodes) succeed; an answer without data fails with the first error.
func (c *Client) graphQL(ctx context.Context, query string, vars map[string]any, out any) error {
	var env struct {
		Data   json.RawMessage `json:"data"`
		Errors []gqlError      `json:"errors"`
	}
	if err := c.do(ctx, http.MethodPost, c.graphqlURL, map[string]any{"query": query, "variables": vars}, &env); err != nil {
		return err
	}
	if len(env.Data) == 0 || string(env.Data) == "null" {
		if len(env.Errors) > 0 {
			return fmt.Errorf("graphql: %s", env.Errors[0].Message)
		}
		return errors.New("graphql: empty response")
	}
	return json.Unmarshal(env.Data, out)
}
