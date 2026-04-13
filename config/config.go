package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const defaultAPIURL = "https://api.usemindex.dev"

// Config armazena as configurações do CLI lidas de ~/.mindex/config.json.
type Config struct {
	APIKey           string `json:"api_key"`
	APIURL           string `json:"api_url"`
	DefaultNamespace string `json:"default_namespace,omitempty"`
	OrgSlug          string `json:"org_slug,omitempty"`
}

// Default retorna uma configuração com os valores padrão.
func Default() *Config {
	return &Config{APIURL: defaultAPIURL}
}

// Path retorna o caminho padrão do arquivo de configuração.
func Path() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".mindex", "config.json")
}

// Load carrega a configuração do caminho padrão.
func Load() (*Config, error) { return LoadFrom(Path()) }

// Save salva a configuração no caminho padrão.
func Save(cfg *Config) error { return SaveTo(cfg, Path()) }

// LoadFrom carrega a configuração a partir de um caminho específico.
// Se o arquivo não existir, retorna a configuração padrão sem erro.
func LoadFrom(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Default(), nil
		}
		return nil, err
	}
	cfg := Default()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// SaveTo salva a configuração em um caminho específico.
// Cria os diretórios intermediários com permissão 0700 e o arquivo com 0600.
func SaveTo(cfg *Config, path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}
