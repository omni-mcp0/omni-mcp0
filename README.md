# Omni MCP

One MCP server that sits between an agent and many upstream MCP servers. The agent sees
only **four meta-tools**, so it can find and call any of thousands of tools without loading
their definitions into context.

> **Status: MVP v0.1 code written (Go, standard library only).** Design docs are in `docs/`.

## The problem

MCP has no cross-server search. An agent connected to 50 servers receives every tool schema
up front — thousands of tools can cost over a million tokens, and tool-selection accuracy
drops long before that.

## The idea

```
Agent ──► Omni MCP ──► github MCP
(4 tools)      │  ───► filesystem MCP
               └─────► logs MCP ...
```

| Meta-tool | Purpose |
|-----------|---------|
| `discover_server` | Find servers matching an intent |
| `discover_tools` | Find tools (inside one server, or across all) |
| `get_tool_schema` | Load one full schema only when needed |
| `invoke_tool` | Execute the call, result passed through unchanged |

Agent context cost stays fixed (~1–2k tokens) no matter how many tools exist behind Omni.

## Quick start

```bash
go build -o omni ./cmd/omni
# edit omni.json to list your upstream servers, then:
./omni -config omni.json
```

Add it to Claude Code as an MCP server whose command is `/path/to/omni -config /path/to/omni.json`.

## Watch the logs (side server + test script)

Terminal 1 — run Omni as a side server (logs print here, and are saved to `omni.log`):

```bash
go run ./cmd/omni -config omni.json -listen :8080 -log-file omni.log
```

Terminal 2 — send test requests; every one shows up in Terminal 1:

```bash
scripts/test.sh
```

Log lines: `-> meta-tool call` (request arrived), `search ...` (what ranked first),
`upstream call` (which real server got it, how many ms), `<- meta-tool done` (finished, `is_error`).
Add `-debug` for more detail.

## Code map (read in this order)

| File | What it does |
|------|--------------|
| `cmd/omni/main.go` | Entry point: load config, connect servers, serve |
| `internal/omni/config.go` | `omni.json` format |
| `internal/omni/upstream.go` | One child-process MCP connection (initialize, tools/list, tools/call) |
| `internal/omni/catalog.go` | In-memory map of servers and tools |
| `internal/omni/search.go` | Keyword ranking (name x3, description x2, rare words count more) |
| `internal/omni/frontend.go` | The 4 meta-tools the agent sees |
| `cmd/fakeserver/` | Tiny fake upstream for local testing |

## Documents

| Doc | What it is |
|-----|------------|
| [docs/MVP_DESIGN.md](docs/MVP_DESIGN.md) ([PDF](docs/Omni-MCP-MVP-Design.pdf)) | Minimal MVP: scope, architecture, validation plan, go/no-go criteria |
| [docs/SYSTEM_DESIGN.md](docs/SYSTEM_DESIGN.md) ([PDF](docs/Omni-MCP-System-Design.pdf)) | Complete v1.0 design in Go: health checks, hybrid search, security, benchmark |

## MVP in one paragraph

Go binary, stdio only. Connects to stdio upstream servers from `omni.json`, keeps
their tools in an in-memory catalog, searches with a simple BM25-style ranker, checks
required arguments and forwards calls. A small benchmark measures Recall@k and token savings. Health checks,
embeddings, persistence, auth and policy come later.

## Roadmap

| Version | Goal |
|---------|------|
| **MVP / v0.1** | Pass-through proxy + BM25 search + mini benchmark → go/no-go |
| v0.2 | SQLite catalog, `list_changed` sync |
| v0.3 | Embeddings + RRF hybrid search |
| v0.4 | Health checker, circuit breaker, hot reload |
| v0.5 | `omni-bench` with published results |
| v0.6 | Policy, approvals, redaction, HTTP auth |
| v1.0 | Projection, metrics, Docker, stable config |

## Progress

- [x] Full system design (Go edition)
- [x] MVP design
- [ ] Design review / approval
- [x] MVP implementation (Go)
- [ ] Benchmark & go/no-go decision

## Tech stack (planned)

Go (standard library only, no external dependencies)

## Contributing

Design is still moving. Open an issue to discuss before sending code.
