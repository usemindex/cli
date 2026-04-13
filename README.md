# Mindex CLI

Give your AI real memory — from the terminal.

## Install

```bash
# Go
go install github.com/usemindex/cli@latest

# Or download from GitHub Releases
# https://github.com/usemindex/cli/releases
```

## Quick Start

```bash
# 1. Authenticate
mindex auth

# 2. Get context from your knowledge base
mindex context "how does authentication work?"
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

Supported formats: `.md`, `.txt`, `.pdf`, `.docx`, `.pptx`, `.xlsx`, `.html`, `.csv`, `.json`, `.xml`

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
