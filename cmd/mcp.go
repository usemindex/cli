package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/spf13/cobra"
	"github.com/usemindex/cli/config"
)

const mcpURL = "https://mcp.usemindex.dev"

// mcpToolInfo descreve um AI tool suportado pelo comando mcp install.
type mcpToolInfo struct {
	nome        string
	configPath  func() string
	exibirCampo string // caminho exibido ao usuário
}

// mcpTools mapeia o identificador da ferramenta para suas informações.
var mcpTools = map[string]*mcpToolInfo{
	"claude-code": {
		nome: "Claude Code",
		configPath: func() string {
			return ".mcp.json"
		},
		exibirCampo: ".mcp.json",
	},
	"cursor": {
		nome: "Cursor",
		configPath: func() string {
			return filepath.Join(".cursor", "mcp.json")
		},
		exibirCampo: ".cursor/mcp.json",
	},
	"windsurf": {
		nome: "Windsurf",
		configPath: func() string {
			home, _ := os.UserHomeDir()
			return filepath.Join(home, ".codeium", "windsurf", "mcp_config.json")
		},
		exibirCampo: "~/.codeium/windsurf/mcp_config.json",
	},
	"claude-desktop": {
		nome: "Claude Desktop",
		configPath: func() string {
			home, _ := os.UserHomeDir()
			switch runtime.GOOS {
			case "darwin":
				return filepath.Join(home, "Library", "Application Support", "Claude", "claude_desktop_config.json")
			case "windows":
				appdata := os.Getenv("APPDATA")
				if appdata == "" {
					appdata = filepath.Join(home, "AppData", "Roaming")
				}
				return filepath.Join(appdata, "Claude", "claude_desktop_config.json")
			default:
				return filepath.Join(home, ".config", "Claude", "claude_desktop_config.json")
			}
		},
		exibirCampo: "claude_desktop_config.json",
	},
}

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Gerencia a integração MCP com ferramentas de IA",
	Long: `Gerencia a integração do servidor MCP do Mindex com ferramentas de IA.

  Ferramentas suportadas:
    claude-code     .mcp.json no diretório atual
    cursor          .cursor/mcp.json no diretório atual
    windsurf        ~/.codeium/windsurf/mcp_config.json
    claude-desktop  arquivo de config do Claude Desktop

  Exemplos:
    mindex mcp install claude-code
    mindex mcp install cursor
    mindex mcp status`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

var mcpInstallCmd = &cobra.Command{
	Use:   "install <tool>",
	Short: "Configura o servidor MCP do Mindex em uma ferramenta de IA",
	Long: `Configura o servidor MCP do Mindex em uma ferramenta de IA.

  Ferramentas suportadas: claude-code, cursor, windsurf, claude-desktop

  Exemplos:
    mindex mcp install claude-code
    mindex mcp install cursor`,
	Args: cobra.ExactArgs(1),
	RunE: runMcpInstall,
}

var mcpStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Mostra quais ferramentas têm o Mindex MCP configurado",
	RunE:  runMcpStatus,
}

func init() {
	mcpCmd.AddCommand(mcpInstallCmd)
	mcpCmd.AddCommand(mcpStatusCmd)
	rootCmd.AddCommand(mcpCmd)
}

func runMcpInstall(cmd *cobra.Command, args []string) error {
	toolKey := args[0]

	tool, ok := mcpTools[toolKey]
	if !ok {
		return fmt.Errorf("ferramenta desconhecida '%s'. Suportadas: claude-code, cursor, windsurf, claude-desktop", toolKey)
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("erro ao carregar configuração: %w", err)
	}
	if cfg.APIKey == "" {
		return fmt.Errorf("não autenticado. Execute 'mindex auth' primeiro.")
	}

	configPath := tool.configPath()

	if err := writeMCPConfig(configPath, cfg.APIKey); err != nil {
		return fmt.Errorf("erro ao escrever configuração: %w", err)
	}

	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "  ✓ Mindex MCP configurado para %s\n", tool.nome)
	fmt.Fprintf(w, "    Config: %s\n", tool.exibirCampo)
	fmt.Fprintf(w, "    Reinicie %s para ativar.\n", tool.nome)

	return nil
}

func runMcpStatus(cmd *cobra.Command, args []string) error {
	w := cmd.OutOrStdout()

	// ordem de exibição
	ordem := []string{"claude-code", "cursor", "windsurf", "claude-desktop"}

	for _, key := range ordem {
		tool := mcpTools[key]
		configPath := tool.configPath()
		configurado := isMCPConfigured(configPath)

		if configurado {
			fmt.Fprintf(w, "  %-16s ✓ configurado (%s)\n", tool.nome+":", tool.exibirCampo)
		} else {
			fmt.Fprintf(w, "  %-16s ✗ não configurado\n", tool.nome+":")
		}
	}

	return nil
}

// writeMCPConfig lê o arquivo de configuração existente (se houver), adiciona/atualiza
// a entrada "mindex" em "mcpServers" preservando todas as outras chaves, e salva.
func writeMCPConfig(path, apiKey string) error {
	// carrega configuração existente ou inicializa um mapa vazio
	existing := map[string]any{}
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("erro ao ler %s: %w", path, err)
	}
	if err == nil && len(data) > 0 {
		if err := json.Unmarshal(data, &existing); err != nil {
			return fmt.Errorf("JSON inválido em %s: %w", path, err)
		}
	}

	// garante que mcpServers existe
	mcpServers, _ := existing["mcpServers"].(map[string]any)
	if mcpServers == nil {
		mcpServers = map[string]any{}
	}

	// adiciona/atualiza apenas a entrada "mindex"
	mcpServers["mindex"] = map[string]any{
		"type": "http",
		"url":  mcpURL,
		"headers": map[string]any{
			"Authorization": "Bearer " + apiKey,
		},
	}
	existing["mcpServers"] = mcpServers

	// cria diretórios intermediários se necessário
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("erro ao criar diretório %s: %w", dir, err)
	}

	// serializa com indentação
	out, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return fmt.Errorf("erro ao serializar configuração: %w", err)
	}

	return os.WriteFile(path, out, 0600)
}

// isMCPConfigured verifica se o arquivo existe e contém a entrada "mindex" em "mcpServers".
func isMCPConfigured(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		return false
	}
	servers, ok := cfg["mcpServers"].(map[string]any)
	if !ok {
		return false
	}
	_, exists := servers["mindex"]
	return exists
}
