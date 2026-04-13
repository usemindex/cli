# Stop using naive RAG

Most RAG systems lose relationships between documents. Mindex doesn't.

See the difference:

```bash
mindex context "how does the payment flow work?" --compare
```

```
  ┌─────────────────────────────────────────────────┐
  │  NAIVE RAG (vector similarity only)             │
  └─────────────────────────────────────────────────┘

  [payment-flow.md] (similarity: 71%)
  The payment system uses Stripe checkout sessions.
  (No mention of webhooks, billing state, or downstream effects)

  ┌─────────────────────────────────────────────────┐
  │  MINDEX GRAPHRAG (similarity + relationships)   │
  └─────────────────────────────────────────────────┘

  === SEMANTIC SEARCH ===
  [payment-flow.md] (relevance: 92%)
  Complete payment flow with Stripe checkout sessions...

  === KNOWLEDGE GRAPH ===
  [billing-setup.md] (relevance: 85%)
  Webhook handling: checkout.session.completed updates subscription
  status, triggers email notification, and syncs plan limits...

  [api-reference.md] (relevance: 78%)
  POST /billing/checkout creates Stripe session with org metadata,
  links customer to subscription, and validates plan limits...

  Sources: payment-flow.md, billing-setup.md (via RELATES_TO), api-reference.md

  ─────────────────────────────────────────────────
  RAG = similarity matching only
  Mindex = similarity + knowledge graph + relationships
```

Mindex builds a memory layer using knowledge graphs, so your AI understands relationships between your documents — not just similarity.

Works with Claude, Cursor, Windsurf and any MCP-compatible tool.

## Try it yourself

```bash
curl -fsSL https://raw.githubusercontent.com/usemindex/cli/main/install.sh | sh
mindex auth
mindex context "your question here" --compare
```

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
  Found 3 relevant sources

  === SEMANTIC SEARCH ===
  [payment-flow.md] (relevance: 0.92)
  The payment system uses Stripe checkout sessions...

  === KNOWLEDGE GRAPH ===
  [billing-setup.md] (relevance: 0.85)
  Webhook handling for subscription updates...

  Sources: payment-flow.md (92%), billing-setup.md (85%), api-reference.md (78%)
```

Pipe to other tools with `--json`:

```bash
mindex context "deployment steps" --json | jq '.formatted_context'
```

## Read a Document

```bash
mindex get docs/guide.md
mindex get guide.md -n docs
mindex get guide.md --json
```

## MCP Integration

Connect Mindex to your AI coding tools with one command. After connecting, your AI assistant will automatically search your knowledge base when answering questions.

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

> **Note:** `mindex auth` automatically updates MCP configs when you change your API key.

## Commands

| Command | Description |
|---------|-------------|
| `mindex context <question>` | **Retrieve context from your knowledge base (GraphRAG)** |
| `mindex search <query>` | Semantic search across documents |
| `mindex get <key>` | Read the full content of a document |
| `mindex upload <file>` | Upload documents (PDF, Word, Markdown, and more) |
| `mindex list` | List documents in a namespace |
| `mindex delete <doc>` | Delete a document |
| `mindex namespaces` | List namespaces |
| `mindex namespaces create <name>` | Create a namespace |
| `mindex mcp install <tool>` | Configure MCP server in AI tools |
| `mindex mcp status` | Check MCP configuration status |
| `mindex auth` | Configure API key (auto-updates MCP configs) |
| `mindex config` | Show current configuration |
| `mindex status` | Check API connectivity |
| `mindex update` | Update CLI to the latest version |
| `mindex --version` | Show current version |

## Upload

Supports multiple formats and batch upload:

```bash
mindex upload README.md
mindex upload *.pdf -n docs
mindex upload src/ --recursive
```

Supported: `.md`, `.txt`, `.pdf`, `.docx`, `.pptx`, `.xlsx`, `.html`, `.csv`, `.json`, `.xml`

## Self-Update

The CLI checks for updates automatically and notifies you:

```
  Update available: v0.3.2 → v0.3.3
  Run: mindex update
```

Or update manually:

```bash
mindex update
```

## Flags

| Flag | Description |
|------|-------------|
| `--json` / `-j` | JSON output for scripts and piping |
| `--namespace` / `-n` | Target namespace |
| `--no-color` | Disable colored output |
| `--quiet` / `-q` | Minimal output |

## Links

- **Website**: [usemindex.dev](https://usemindex.dev)
- **API Docs**: [github.com/usemindex/docs](https://github.com/usemindex/docs)
- **MCP Server**: [github.com/usemindex/mcp](https://github.com/usemindex/mcp)
- **Support**: support@usemindex.dev
