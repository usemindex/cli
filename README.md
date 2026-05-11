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

Bulk upload sends up to 50 files per HTTP request (instead of one request per file), so a thousand-file upload finishes in seconds and stays well under per-org rate limits.

```bash
mindex upload README.md
mindex upload *.pdf -n docs
mindex upload src/ --recursive
mindex upload **/*.md -n knowledge-base    # 1000+ files just works
```

What you'll see:

```
Uploading 115 files in 3 batches...
  ✓ Batch 1/3 enqueued (50 files)
  ✓ Batch 2/3 enqueued (50 files)
  ✓ Batch 3/3 enqueued (15 files)
  Processing... 110/115
Done: 115 indexed, 0 failed (12.4s)
```

Supported: `.md`, `.txt`, `.pdf`, `.docx`, `.pptx`, `.xlsx`, `.html`, `.csv`, `.json`, `.xml`. Max 10 MB per file.

If a single file fails validation (unsupported type, exceeds 10 MB, invalid encoding), **the whole batch is rejected** with no files uploaded — atomic by design. Document count is checked atomically across the batch; if the total would exceed your plan's limit, the upload is cancelled with a clear error before anything is sent.

Press `Ctrl+C` while files are processing — uploads continue in the background; you can re-poll later with `mindex status <task_id>`.

See [Plan limits](#plan-limits) for the full breakdown of per-plan document count and per-file markdown caps.

## Plan limits

Mindex enforces two independent size limits on every upload:

| Plan | Max documents | Max markdown per document | Max raw file |
|------|---------------|---------------------------|--------------|
| Free | 100 | 30 KB | 10 MB |
| Personal | 5,000 | 150 KB | 10 MB |
| Team | 15,000 | 300 KB | 10 MB |
| Enterprise | Tailored | Tailored | 10 MB |

For Enterprise plans, all limits are negotiated individually — [contact us](mailto:support@usemindex.dev).

### Why two size limits?

We convert every file (PDF, DOCX, etc.) to markdown before processing. The **raw** file size protects against catastrophic uploads. The **markdown** size — the actual cost driver — is what the plan limits. A 5 MB PDF might produce only 100 KB of markdown text; a 5 MB plain `.md` file is 5 MB of markdown.

If you're hitting the markdown cap with `.md` uploads, consider splitting documents into smaller logical units (chapters, sections).

## Errors you might see

| Error | What it means | What to do |
|-------|---------------|------------|
| `MARKDOWN_TOO_LARGE` | A document's converted markdown exceeds your plan's per-doc limit | Split the doc into smaller files, or upgrade your plan |
| `DOCUMENT_EXISTS` | A doc with the same key already exists in this namespace | Use `--overwrite` to replace, or upload to a different namespace |
| `402 Document limit reached` | You hit your plan's document count cap | Delete some docs or upgrade |
| `429 Rate limited` | Per-minute request cap hit | The CLI retries automatically — just wait |
| `413 Payload too large` | Single-file upload exceeded plan markdown limit | Split or upgrade |

When a batch contains some failing files, the CLI continues with the rest and shows a grouped summary at the end. No need to re-run the whole upload.

## Checking your usage

```bash
# Via dashboard
open https://usemindex.dev/orgs/<your-org-slug>/settings/billing

# Via API (shows current doc count, max, and all limits)
curl -H "Authorization: $MINDEX_API_KEY" \
  https://api.usemindex.dev/orgs/<your-org-slug>/billing/subscription
```

## Commands

| Command | Description |
|---------|-------------|
| `mindex context <question>` | **Retrieve context from your knowledge base (GraphRAG)** |
| `mindex search <query>` | Semantic search across documents |
| `mindex get <key>` | Read the full content of a document |
| `mindex related <key>` | Find documents related via shared entities |
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
