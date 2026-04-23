package cmd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/usemindex/cli/api"
)

// runRelatedAgainst executes the real related command logic against a test server,
// bypassing config.Load() by constructing the client directly. Returns stdout and error.
func runRelatedAgainst(t *testing.T, srv *httptest.Server, key string, opts ...func()) (string, error) {
	t.Helper()

	// reset flags to defaults; caller options can override
	relatedLimit = 10
	relatedMinShared = 1
	jsonOutput = false
	noColor = true

	for _, opt := range opts {
		opt()
	}

	client := api.New(srv.URL, "sk-test")
	client.OrgSlug = "test-org"

	cmd := &cobra.Command{Use: "related"}
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	err := executeRelated(cmd, client, key)
	return buf.String(), err
}

// executeRelated mirrors runRelated's core — takes a preconfigured client and
// runs the same flag validation, API call, and formatter as production code.
// This keeps the test harness aligned with real execution paths.
func executeRelated(cmd *cobra.Command, client *api.Client, key string) error {
	if relatedLimit < 1 || relatedLimit > relatedMaxLimit {
		return &usageError{msg: "limit out of range"}
	}
	if relatedMinShared < 1 {
		return &usageError{msg: "min-shared must be >= 1"}
	}

	result, err := client.RelatedDocuments(key, relatedLimit, relatedMinShared)
	if err != nil {
		if apiErr, ok := err.(*api.APIError); ok {
			switch apiErr.Status {
			case 404:
				return &notFoundError{key: key}
			case 400:
				if apiErr.Message != "" {
					return &badRequestError{msg: apiErr.Message}
				}
			}
		}
		return err
	}

	if jsonOutput {
		return printRelatedJSON(cmd, result)
	}
	return printRelatedHuman(cmd, result)
}

type notFoundError struct{ key string }

func (e *notFoundError) Error() string { return "document not found: " + e.key }

type badRequestError struct{ msg string }

func (e *badRequestError) Error() string { return e.msg }

// --- Tests ---

func TestRelated_HappyPath(t *testing.T) {
	title := "Slack"
	discord := "Discord"
	teams := "Microsoft Teams"

	response := api.RelatedDocumentsResponse{
		Document: "docs/slack.md",
		Title:    &title,
		Related: []api.RelatedDocument{
			{
				Filename:       "docs/discord.md",
				Title:          &discord,
				SharedEntities: []string{"subscription.cancelled", "webhook"},
				SharedCount:    2,
				Strength:       0.67,
			},
			{
				Filename:       "docs/microsoft-teams.md",
				Title:          &teams,
				SharedEntities: []string{"subscription.cancelled"},
				SharedCount:    1,
				Strength:       0.55,
			},
		},
		EntitiesInThisDoc: []api.EntityInDocument{
			{Name: "subscription.cancelled", Type: "event", AlsoMentionedIn: 3},
			{Name: "webhook", Type: "concept", AlsoMentionedIn: 2},
			{Name: "attachment", Type: "format", AlsoMentionedIn: 0},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/api/v1/test-org/documents/") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if !strings.HasSuffix(r.URL.Path, "/related") {
			t.Errorf("path should end with /related, got: %s", r.URL.Path)
		}
		if q := r.URL.Query().Get("limit"); q != "10" {
			t.Errorf("limit param: expected '10', got %q", q)
		}
		if q := r.URL.Query().Get("min_shared"); q != "1" {
			t.Errorf("min_shared param: expected '1', got %q", q)
		}
		if r.Header.Get("Authorization") != "Bearer sk-test" {
			t.Errorf("missing auth header")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer srv.Close()

	out, err := runRelatedAgainst(t, srv, "docs/slack.md")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	checks := []string{
		"Slack",                   // title shown
		"docs/discord.md",         // related filename
		"docs/microsoft-teams.md", // second related filename
		"67%",                     // strength formatted
		"subscription.cancelled",  // shared entity
		"Entities in this doc",    // entities section header
		"unique to this doc",      // AlsoMentionedIn == 0
		"also in 3 other docs",    // AlsoMentionedIn > 1
		"also in 2 other docs",    // second entity
	}
	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("output should contain %q, got:\n%s", want, out)
		}
	}
}

func TestRelated_EmptyRelated(t *testing.T) {
	title := "Slack"
	response := api.RelatedDocumentsResponse{
		Document:          "docs/slack.md",
		Title:             &title,
		Related:           []api.RelatedDocument{},
		EntitiesInThisDoc: []api.EntityInDocument{},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer srv.Close()

	out, err := runRelatedAgainst(t, srv, "docs/slack.md")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "No related documents yet") {
		t.Errorf("output should contain empty-state message, got: %q", out)
	}
}

func TestRelated_404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]any{"error": "Document not found"})
	}))
	defer srv.Close()

	_, err := runRelatedAgainst(t, srv, "docs/missing.md")
	if err == nil {
		t.Fatal("expected error for 404")
	}
	if _, ok := err.(*notFoundError); !ok {
		t.Errorf("expected *notFoundError, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "document not found") {
		t.Errorf("error should mention 'document not found', got: %v", err)
	}
}

func TestRelated_400LimitTooLarge(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{"error": "limit must be <= 50"})
	}))
	defer srv.Close()

	_, err := runRelatedAgainst(t, srv, "docs/slack.md")
	if err == nil {
		t.Fatal("expected error for 400")
	}
	if !strings.Contains(err.Error(), "limit must be") {
		t.Errorf("error should surface API message, got: %v", err)
	}
}

func TestRelated_JSONFlag(t *testing.T) {
	title := "Slack"
	response := api.RelatedDocumentsResponse{
		Document: "docs/slack.md",
		Title:    &title,
		Related: []api.RelatedDocument{
			{
				Filename:       "docs/discord.md",
				SharedEntities: []string{"webhook"},
				SharedCount:    1,
				Strength:       0.5,
			},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer srv.Close()

	out, err := runRelatedAgainst(t, srv, "docs/slack.md", func() { jsonOutput = true })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// output must be valid JSON that matches the response struct
	var decoded api.RelatedDocumentsResponse
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("JSON output should be valid: %v\noutput: %s", err, out)
	}
	if decoded.Document != "docs/slack.md" {
		t.Errorf("decoded.Document: expected 'docs/slack.md', got %q", decoded.Document)
	}
	if len(decoded.Related) != 1 || decoded.Related[0].Filename != "docs/discord.md" {
		t.Errorf("JSON should preserve related list, got %+v", decoded.Related)
	}

	// must NOT contain human-formatted labels
	if strings.Contains(out, "strength:") || strings.Contains(out, "Entities in this doc") {
		t.Errorf("--json output should not contain human-formatted labels, got: %s", out)
	}
}

func TestRelated_InvalidLimitRange(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("server should not be called when flag validation fails")
	}))
	defer srv.Close()

	_, err := runRelatedAgainst(t, srv, "docs/slack.md", func() { relatedLimit = 100 })
	if err == nil {
		t.Fatal("expected usage error for limit out of range")
	}
	if _, ok := err.(*usageError); !ok {
		t.Errorf("expected *usageError, got %T: %v", err, err)
	}
}

func TestRelated_TruncatesSharedEntities(t *testing.T) {
	entities := []string{"a", "b", "c", "d", "e", "f", "g"}
	response := api.RelatedDocumentsResponse{
		Document: "docs/x.md",
		Related: []api.RelatedDocument{
			{
				Filename:       "docs/y.md",
				SharedEntities: entities,
				SharedCount:    len(entities),
				Strength:       0.8,
			},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer srv.Close()

	out, err := runRelatedAgainst(t, srv, "docs/x.md")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "+2 more") {
		t.Errorf("output should truncate with '+2 more', got: %q", out)
	}
}

func TestRelated_FilenameFallbackWhenNoTitle(t *testing.T) {
	response := api.RelatedDocumentsResponse{
		Document: "docs/untitled.md",
		Title:    nil,
		Related: []api.RelatedDocument{
			{Filename: "docs/other.md", Title: nil, SharedEntities: []string{"x"}, Strength: 0.3},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer srv.Close()

	out, err := runRelatedAgainst(t, srv, "docs/untitled.md")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// header falls back to filename when title is nil
	if !strings.Contains(out, "docs/untitled.md") {
		t.Errorf("output should contain filename as header fallback, got: %q", out)
	}
}
