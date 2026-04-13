# Mindex CLI

Give your AI real memory — from the terminal.

## Install

```bash
# macOS / Linux
curl -fsSL https://raw.githubusercontent.com/usemindex/cli/main/install.sh | sh

# Windows (PowerShell)
irm https://github.com/usemindex/cli/releases/latest/download/mindex_windows_amd64.zip -OutFile m.zip; Expand-Archive m.zip; mv m\mindex.exe $env:LOCALAPPDATA\Microsoft\WindowsApps\; rm m.zip,m -r

# Go
go install github.com/usemindex/cli@latest
```

> All binaries at [GitHub Releases](https://github.com/usemindex/cli/releases)

## Quick Start

```bash
# 1. Authenticate
mindex auth

# 2. Get context from your knowledge base
mindex context "how does authentication work?"

# 3. Connect to your AI tool
mindex mcp install claude-code
```

## The `context` Command

The core feature of Mindex — retrieve enriched context from your documents using semantic search and knowledge graph traversal (GraphRAG).

```bash
mindex context "how does the payment flow work?"
```

```
  Found 5 relevant results

  ## Payment Flow
  The payment system uses Stripe checkout sessions...

  ## Webhook Handling
  When a payment succeeds, the webhook updates...

  Sources: payment-guide.md (95%), stripe-setup.md (87%)
```

Pipe to other tools with `--json`:

```bash
mindex context "deployment steps" --json | jq '.results[].content'
```

## MCP Integration

Connect Mindex to your AI coding tools with one command:

```bash
mindex mcp install claude-code      # Claude Code
mindex mcp install cursor           # Cursor
mindex mcp install windsurf         # Windsurf
mindex mcp install claude-desktop   # Claude Desktop
```

Check which tools are configured:

```bash
mindex mcp status
```

## Commands

| Command | Description |
|---------|-------------|
| `mindex context <question>` | **Retrieve context from your knowledge base** |
| `mindex search <query>` | Semantic search |
| `mindex upload <file>` | Upload documents (PDF, Word, Markdown, and more) |
| `mindex list` | List documents |
| `mindex delete <doc>` | Delete a document |
| `mindex namespaces` | List namespaces |
| `mindex namespaces create <name>` | Create a namespace |
| `mindex mcp install <tool>` | Configure MCP server in AI tools |
| `mindex mcp status` | Check MCP configuration status |
| `mindex auth` | Configure API key |
| `mindex config` | Show configuration |
| `mindex status` | Check connectivity |

## Upload

Supports multiple formats and batch upload:

```bash
mindex upload README.md
mindex upload *.pdf -n docs
mindex upload src/ --recursive
```

Supported: `.md`, `.txt`, `.pdf`, `.docx`, `.pptx`, `.xlsx`, `.html`, `.csv`, `.json`, `.xml`

## Flags

| Flag | Description |
|------|-------------|
| `--json` / `-j` | JSON output for scripts |
| `--namespace` / `-n` | Target namespace |
| `--no-color` | Disable colors |
| `--quiet` / `-q` | Minimal output |

## Links

- **Website**: [usemindex.dev](https://usemindex.dev)
- **API Docs**: [github.com/usemindex/docs](https://github.com/usemindex/docs)
- **MCP Server**: [github.com/usemindex/mcp](https://github.com/usemindex/mcp)
- **Support**: support@usemindex.dev
