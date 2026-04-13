package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// --- writeMCPConfig ---

func TestWriteMCPConfig_NovoArquivo(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".mcp.json")

	if err := writeMCPConfig(path, "sk-test-key"); err != nil {
		t.Fatalf("writeMCPConfig() erro: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("erro ao ler arquivo criado: %v", err)
	}

	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("JSON inválido: %v", err)
	}

	servers, ok := cfg["mcpServers"].(map[string]any)
	if !ok {
		t.Fatal("mcpServers ausente ou tipo errado")
	}

	mindex, ok := servers["mindex"].(map[string]any)
	if !ok {
		t.Fatal("entrada 'mindex' ausente em mcpServers")
	}

	if mindex["type"] != "http" {
		t.Errorf("type: esperado 'http', obtido %q", mindex["type"])
	}
	if mindex["url"] != mcpURL {
		t.Errorf("url: esperado %q, obtido %q", mcpURL, mindex["url"])
	}

	headers, ok := mindex["headers"].(map[string]any)
	if !ok {
		t.Fatal("headers ausente ou tipo errado")
	}
	if headers["Authorization"] != "Bearer sk-test-key" {
		t.Errorf("Authorization: esperado 'Bearer sk-test-key', obtido %q", headers["Authorization"])
	}
}

func TestWriteMCPConfig_MergeArquivoExistente(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.json")

	// arquivo existente com outro servidor configurado
	existente := map[string]any{
		"mcpServers": map[string]any{
			"outro-servidor": map[string]any{
				"type": "stdio",
				"command": "outro-bin",
			},
		},
		"outraChave": "valor-preservado",
	}
	data, _ := json.Marshal(existente)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatalf("erro ao criar arquivo de teste: %v", err)
	}

	if err := writeMCPConfig(path, "sk-nova-key"); err != nil {
		t.Fatalf("writeMCPConfig() erro: %v", err)
	}

	result, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("erro ao ler arquivo resultante: %v", err)
	}

	var cfg map[string]any
	if err := json.Unmarshal(result, &cfg); err != nil {
		t.Fatalf("JSON inválido: %v", err)
	}

	// chave de nível superior preservada
	if cfg["outraChave"] != "valor-preservado" {
		t.Errorf("outraChave deveria ser preservada, obtido %v", cfg["outraChave"])
	}

	servers, ok := cfg["mcpServers"].(map[string]any)
	if !ok {
		t.Fatal("mcpServers ausente ou tipo errado")
	}

	// servidor existente preservado
	if _, ok := servers["outro-servidor"]; !ok {
		t.Error("'outro-servidor' deveria ter sido preservado")
	}

	// entrada mindex adicionada
	mindex, ok := servers["mindex"].(map[string]any)
	if !ok {
		t.Fatal("entrada 'mindex' ausente em mcpServers")
	}

	headers, _ := mindex["headers"].(map[string]any)
	if headers["Authorization"] != "Bearer sk-nova-key" {
		t.Errorf("Authorization incorreto: %v", headers["Authorization"])
	}
}

func TestWriteMCPConfig_AtualizaEntradaExistente(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.json")

	// arquivo com mindex já configurado com key antiga
	existente := map[string]any{
		"mcpServers": map[string]any{
			"mindex": map[string]any{
				"type": "http",
				"url":  mcpURL,
				"headers": map[string]any{
					"Authorization": "Bearer sk-key-antiga",
				},
			},
		},
	}
	data, _ := json.Marshal(existente)
	os.WriteFile(path, data, 0600)

	if err := writeMCPConfig(path, "sk-key-nova"); err != nil {
		t.Fatalf("writeMCPConfig() erro: %v", err)
	}

	result, _ := os.ReadFile(path)
	var cfg map[string]any
	json.Unmarshal(result, &cfg)

	servers := cfg["mcpServers"].(map[string]any)
	mindex := servers["mindex"].(map[string]any)
	headers := mindex["headers"].(map[string]any)

	if headers["Authorization"] != "Bearer sk-key-nova" {
		t.Errorf("chave deveria ter sido atualizada, obtido %v", headers["Authorization"])
	}
}

func TestWriteMCPConfig_CriaSubdiretorio(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".cursor", "mcp.json")

	if err := writeMCPConfig(path, "sk-key"); err != nil {
		t.Fatalf("writeMCPConfig() deveria criar subdiretório, erro: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Errorf("arquivo não foi criado em subdiretório: %v", err)
	}
}

func TestWriteMCPConfig_SemMcpServersNoArquivoExistente(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	// arquivo sem mcpServers
	existente := map[string]any{"outraConfig": true}
	data, _ := json.Marshal(existente)
	os.WriteFile(path, data, 0600)

	if err := writeMCPConfig(path, "sk-key"); err != nil {
		t.Fatalf("writeMCPConfig() erro: %v", err)
	}

	result, _ := os.ReadFile(path)
	var cfg map[string]any
	json.Unmarshal(result, &cfg)

	if _, ok := cfg["mcpServers"]; !ok {
		t.Error("mcpServers deveria ter sido criado")
	}
	if cfg["outraConfig"] != true {
		t.Error("outraConfig deveria ter sido preservada")
	}
}

// --- isMCPConfigured ---

func TestIsMCPConfigured_Configurado(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.json")

	writeMCPConfig(path, "sk-key")

	if !isMCPConfigured(path) {
		t.Error("isMCPConfigured() deveria retornar true após writeMCPConfig()")
	}
}

func TestIsMCPConfigured_NaoConfigurado(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.json")

	existente := map[string]any{
		"mcpServers": map[string]any{
			"outro": map[string]any{"type": "stdio"},
		},
	}
	data, _ := json.Marshal(existente)
	os.WriteFile(path, data, 0600)

	if isMCPConfigured(path) {
		t.Error("isMCPConfigured() deveria retornar false quando 'mindex' não existe em mcpServers")
	}
}

func TestIsMCPConfigured_ArquivoInexistente(t *testing.T) {
	if isMCPConfigured("/caminho/que/nao/existe/mcp.json") {
		t.Error("isMCPConfigured() deveria retornar false para arquivo inexistente")
	}
}

// --- resolução de caminhos ---

func TestMcpToolsPathResolution(t *testing.T) {
	home, _ := os.UserHomeDir()

	testes := []struct {
		chave         string
		verificar     func(string) bool
		descricao     string
	}{
		{
			chave: "claude-code",
			verificar: func(p string) bool { return p == ".mcp.json" },
			descricao: "deve ser .mcp.json no diretório atual",
		},
		{
			chave: "cursor",
			verificar: func(p string) bool {
				return strings.HasSuffix(filepath.ToSlash(p), ".cursor/mcp.json")
			},
			descricao: "deve ser .cursor/mcp.json no diretório atual",
		},
		{
			chave: "windsurf",
			verificar: func(p string) bool {
				esperado := filepath.Join(home, ".codeium", "windsurf", "mcp_config.json")
				return p == esperado
			},
			descricao: "deve ser ~/.codeium/windsurf/mcp_config.json",
		},
		{
			chave: "claude-desktop",
			verificar: func(p string) bool {
				switch runtime.GOOS {
				case "darwin":
					return p == filepath.Join(home, "Library", "Application Support", "Claude", "claude_desktop_config.json")
				case "windows":
					appdata := os.Getenv("APPDATA")
					if appdata == "" {
						appdata = filepath.Join(home, "AppData", "Roaming")
					}
					return p == filepath.Join(appdata, "Claude", "claude_desktop_config.json")
				default:
					return p == filepath.Join(home, ".config", "Claude", "claude_desktop_config.json")
				}
			},
			descricao: "deve resolver o caminho correto conforme o SO",
		},
	}

	for _, tc := range testes {
		t.Run(tc.chave, func(t *testing.T) {
			tool, ok := mcpTools[tc.chave]
			if !ok {
				t.Fatalf("ferramenta '%s' não encontrada em mcpTools", tc.chave)
			}
			path := tool.configPath()
			if !tc.verificar(path) {
				t.Errorf("%s: caminho obtido %q não satisfaz verificação (%s)", tc.chave, path, tc.descricao)
			}
		})
	}
}

func TestMcpTools_TodasFeramentasPresentes(t *testing.T) {
	esperadas := []string{"claude-code", "cursor", "windsurf", "claude-desktop"}
	for _, key := range esperadas {
		if _, ok := mcpTools[key]; !ok {
			t.Errorf("ferramenta '%s' não encontrada em mcpTools", key)
		}
	}
}
