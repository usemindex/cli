package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefault(t *testing.T) {
	cfg := Default()
	if cfg == nil {
		t.Fatal("Default() retornou nil")
	}
	if cfg.APIURL != defaultAPIURL {
		t.Errorf("APIURL esperado %q, obtido %q", defaultAPIURL, cfg.APIURL)
	}
	if cfg.APIKey != "" {
		t.Errorf("APIKey deveria ser vazio, obtido %q", cfg.APIKey)
	}
}

func TestSaveToAndLoadFrom(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	original := &Config{
		APIKey:           "sk-test-key",
		APIURL:           "https://api.example.com",
		DefaultNamespace: "minha-base",
		OrgSlug:          "minha-org",
	}

	if err := SaveTo(original, path); err != nil {
		t.Fatalf("SaveTo() erro: %v", err)
	}

	loaded, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom() erro: %v", err)
	}

	if loaded.APIKey != original.APIKey {
		t.Errorf("APIKey: esperado %q, obtido %q", original.APIKey, loaded.APIKey)
	}
	if loaded.APIURL != original.APIURL {
		t.Errorf("APIURL: esperado %q, obtido %q", original.APIURL, loaded.APIURL)
	}
	if loaded.DefaultNamespace != original.DefaultNamespace {
		t.Errorf("DefaultNamespace: esperado %q, obtido %q", original.DefaultNamespace, loaded.DefaultNamespace)
	}
	if loaded.OrgSlug != original.OrgSlug {
		t.Errorf("OrgSlug: esperado %q, obtido %q", original.OrgSlug, loaded.OrgSlug)
	}
}

func TestLoadFromMissingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "inexistente.json")

	cfg, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom() com arquivo inexistente deveria retornar default, erro: %v", err)
	}
	if cfg == nil {
		t.Fatal("LoadFrom() retornou nil para arquivo inexistente")
	}
	if cfg.APIURL != defaultAPIURL {
		t.Errorf("APIURL esperado %q, obtido %q", defaultAPIURL, cfg.APIURL)
	}
}

func TestFilePermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := Default()
	cfg.APIKey = "sk-secret"

	if err := SaveTo(cfg, path); err != nil {
		t.Fatalf("SaveTo() erro: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() erro: %v", err)
	}

	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("permissão esperada 0600, obtida %04o", perm)
	}
}

func TestSaveToCreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "nested", "config.json")

	cfg := Default()
	if err := SaveTo(cfg, path); err != nil {
		t.Fatalf("SaveTo() deveria criar diretórios intermediários, erro: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Errorf("arquivo não foi criado: %v", err)
	}
}

func TestLoadFromInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	if err := os.WriteFile(path, []byte("json inválido {{{"), 0600); err != nil {
		t.Fatalf("erro ao criar arquivo de teste: %v", err)
	}

	_, err := LoadFrom(path)
	if err == nil {
		t.Error("LoadFrom() deveria retornar erro para JSON inválido")
	}
}
