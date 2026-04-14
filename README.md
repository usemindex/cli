# Stop using naive RAG

RAG loses relationships between documents. Mindex keeps them.

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
  ❌ RAG = similarity only
  ✅ Mindex = similarity + knowledge graph + relationships
```

Works with Claude, Cursor, Windsurf and any MCP-compatible tool.

## Try it yourself

```bash
# Install (macOS / Linux)
curl -fsSL https://raw.githubusercontent.com/usemindex/cli/main/install.sh | sh

# Authenticate
mindex auth

# See it in action
mindex context "your question here" --compare
```

<details>
<summary>Other install methods</summary>

```bash
# Windows (PowerShell)
irm https://github.com/usemindex/cli/releases/latest/download/mindex_windows_amd64.zip -OutFile m.zip; Expand-Archive m.zip; mv m\mindex.exe $env:LOCALAPPDATA\Microsoft\WindowsApps\; rm m.zip,m -r

# Go
go install github.com/usemindex/cli@latest
```

> All binaries at [GitHub Releases](https://github.com/usemindex/cli/releases)

</details>

## Quick Start

```bash
# Get context from your knowledge base
mindex context "how does authentication work?"

# Connect to your AI tool
mindex mcp install claude-code
```

## The `context` Command

The core feature — retrieve enriched context using semantic search and knowledge graph traversal (GraphRAG).

```bash
mindex context "how does the payment flow work?"
```

Pipe to other tools with `--json`:

```bash
mindex context "deployment steps" --json | jq '.formatted_context'
```

## MCP Integration

Connect Mindex to your AI coding tools with one command:

```bash
mindex mcp install claude-code      # Claude Code
mindex mcp install cursor           # Cursor
mindex mcp install windsurf         # Windsurf
mindex mcp install claude-desktop   # Claude Desktop
```

> `mindex auth` automatically updates MCP configs when you change your API key.

## AI Skills

Install reusable AI prompts that teach your coding tools how to work with Mindex:

```bash
mindex skills install claude-code   # Install for Claude Code
mindex skills install cursor        # Install for Cursor
mindex skills install windsurf      # Install for Windsurf
```

| Skill | What it does |
|-------|-------------|
| `write-docs` | Write documentation optimized for knowledge graphs |
| `organize-docs` | Suggest namespaces and file organization |
| `troubleshoot` | Search knowledge base for errors before using training data |
| `onboarding` | Build learning paths from connected documents |
| `audit-docs` | Find duplicates, gaps, orphans, and stale content |

```bash
mindex skills list                  # See available skills
mindex skills get write-docs        # Print skill content to stdout
mindex skills update                # Update installed skills
```

## Upload

Supports multiple formats and batch upload:

```bash
mindex upload README.md
mindex upload *.pdf -n docs
mindex upload src/ --recursive
```

Supported: `.md`, `.txt`, `.pdf`, `.docx`, `.pptx`, `.xlsx`, `.html`, `.csv`, `.json`, `.xml`

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
| `mindex skills install <tool>` | Install AI skills for Claude Code, Cursor, or Windsurf |
| `mindex skills list` | List available skills and install status |
| `mindex skills get <skill>` | Print a skill's content to stdout |
| `mindex skills update` | Update installed skills to latest version |
| `mindex auth` | Configure API key (auto-updates MCP configs) |
| `mindex config` | Show current configuration |
| `mindex status` | Check API connectivity |
| `mindex update` | Update CLI to the latest version |

## Flags

| Flag | Description |
|------|-------------|
| `--json` / `-j` | JSON output for scripts and piping |
| `--namespace` / `-n` | Target namespace |
| `--no-color` | Disable colored output |
| `--quiet` / `-q` | Minimal output |

---

If this is useful, [give it a star](https://github.com/usemindex/cli) :star:

## Links

- **Website**: [usemindex.dev](https://usemindex.dev)
- **API Docs**: [github.com/usemindex/docs](https://github.com/usemindex/docs)
- **MCP Server**: [github.com/usemindex/mcp](https://github.com/usemindex/mcp)
- **Support**: support@usemindex.dev
