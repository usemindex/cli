package cmd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// mockRelease constrói um payload de release GitHub para uso nos testes.
func mockRelease(tagName string, assets []map[string]any) map[string]any {
	return map[string]any{
		"tag_name": tagName,
		"assets":   assets,
	}
}

func TestNormalizeVersion(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"v0.1.1", "0.1.1"},
		{"0.1.1", "0.1.1"},
		{"v1.0.0", "1.0.0"},
		{"dev", "dev"},
	}

	for _, tc := range tests {
		got := normalizeVersion(tc.input)
		if got != tc.expected {
			t.Errorf("normalizeVersion(%q) = %q; want %q", tc.input, got, tc.expected)
		}
	}
}

func TestFetchLatestRelease_Success(t *testing.T) {
	release := mockRelease("v0.2.0", []map[string]any{
		{"name": "mindex_linux_amd64.tar.gz", "browser_download_url": "https://example.com/mindex_linux_amd64.tar.gz"},
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(release)
	}))
	defer srv.Close()

	result, err := fetchLatestRelease(srv.URL)
	if err != nil {
		t.Fatalf("fetchLatestRelease() error: %v", err)
	}

	if result["tag_name"] != "v0.2.0" {
		t.Errorf("tag_name: expected 'v0.2.0', got %v", result["tag_name"])
	}
}

func TestFetchLatestRelease_NetworkError(t *testing.T) {
	_, err := fetchLatestRelease("http://127.0.0.1:0/invalid")
	if err == nil {
		t.Error("fetchLatestRelease() should return error on network failure")
	}
}

func TestFetchLatestRelease_NonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := fetchLatestRelease(srv.URL)
	if err == nil {
		t.Error("fetchLatestRelease() should return error on non-200 status")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("error should mention status 404, got: %v", err)
	}
}

func TestFindAsset_MatchingAsset(t *testing.T) {
	// monta um release com o asset correto para o OS/arch atual
	goos := "linux"
	goarch := "amd64"
	assetName := "mindex_" + goos + "_" + goarch + ".tar.gz"
	downloadURL := "https://example.com/" + assetName

	release := map[string]any{
		"tag_name": "v0.2.0",
		"assets": []any{
			map[string]any{
				"name":                 assetName,
				"browser_download_url": downloadURL,
			},
		},
	}

	// sobrescreve os valores de OS/ARCH para testar independente do ambiente
	// testamos a lógica passando o release com o asset exato esperado
	url, name, err := findAssetForPlatform(release, goos, goarch)
	if err != nil {
		t.Fatalf("findAssetForPlatform() error: %v", err)
	}
	if url != downloadURL {
		t.Errorf("url: expected %q, got %q", downloadURL, url)
	}
	if name != assetName {
		t.Errorf("name: expected %q, got %q", assetName, name)
	}
}

func TestFindAsset_NoMatchingAsset(t *testing.T) {
	release := map[string]any{
		"tag_name": "v0.2.0",
		"assets": []any{
			map[string]any{
				"name":                 "mindex_linux_amd64.tar.gz",
				"browser_download_url": "https://example.com/mindex_linux_amd64.tar.gz",
			},
		},
	}

	_, _, err := findAssetForPlatform(release, "freebsd", "386")
	if err == nil {
		t.Error("findAssetForPlatform() should return error when no asset matches")
	}
	if !strings.Contains(err.Error(), "freebsd/386") {
		t.Errorf("error should mention the OS/arch, got: %v", err)
	}
}

func TestFindAsset_EmptyAssets(t *testing.T) {
	release := map[string]any{
		"tag_name": "v0.2.0",
		"assets":   []any{},
	}

	_, _, err := findAssetForPlatform(release, "linux", "amd64")
	if err == nil {
		t.Error("findAssetForPlatform() should return error when assets list is empty")
	}
}

func TestFindAsset_WindowsUsesZip(t *testing.T) {
	release := map[string]any{
		"tag_name": "v0.2.0",
		"assets": []any{
			map[string]any{
				"name":                 "mindex_windows_amd64.zip",
				"browser_download_url": "https://example.com/mindex_windows_amd64.zip",
			},
		},
	}

	url, name, err := findAssetForPlatform(release, "windows", "amd64")
	if err != nil {
		t.Fatalf("findAssetForPlatform() error: %v", err)
	}
	if !strings.HasSuffix(name, ".zip") {
		t.Errorf("Windows asset should be .zip, got %q", name)
	}
	if url == "" {
		t.Error("url should not be empty")
	}
}

func TestRunUpdate_AlreadyLatest(t *testing.T) {
	const currentVer = "v0.1.1"
	Version = currentVer

	release := mockRelease(currentVer, []map[string]any{
		{"name": "mindex_linux_amd64.tar.gz", "browser_download_url": "https://example.com/x.tar.gz"},
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(release)
	}))
	defer srv.Close()

	var buf bytes.Buffer
	cmd := updateCmd
	cmd.SetOut(&buf)

	// invoca a lógica diretamente com o URL do mock
	err := runUpdateWithURL(cmd, nil, srv.URL)
	if err != nil {
		t.Fatalf("runUpdateWithURL() unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Already up to date") {
		t.Errorf("expected 'Already up to date' in output, got: %q", output)
	}
	if !strings.Contains(output, currentVer) {
		t.Errorf("expected version %q in output, got: %q", currentVer, output)
	}
}

func TestRunUpdate_InvalidTagName(t *testing.T) {
	Version = "v0.1.0"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// resposta sem tag_name
		json.NewEncoder(w).Encode(map[string]any{"assets": []any{}})
	}))
	defer srv.Close()

	var buf bytes.Buffer
	cmd := updateCmd
	cmd.SetOut(&buf)

	err := runUpdateWithURL(cmd, nil, srv.URL)
	if err == nil {
		t.Error("runUpdateWithURL() should return error for invalid tag_name")
	}
	if !strings.Contains(err.Error(), "invalid response") {
		t.Errorf("error should mention 'invalid response', got: %v", err)
	}
}
