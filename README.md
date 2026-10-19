# Omni MCP

One MCP server that sits between an agent and many upstream MCP servers. The agent sees
only **four meta-tools**, so it can find and call any of thousands of tools without loading
their definitions into context.

> **Status: design phase.** No code yet. The MVP design is complete and awaiting review.

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

## Documents

| Doc | What it is |
|-----|------------|
| [docs/MVP_DESIGN.md](docs/MVP_DESIGN.md) | Minimal MVP: scope, architecture, validation plan, go/no-go criteria |
| [docs/Omni-MCP-System-Design.pdf](docs/Omni-MCP-System-Design.pdf) | Complete v1.0 design: health checks, hybrid search, security, benchmark |

## MVP in one paragraph

Rust binary, stdio only. Connects to 2–3 stdio upstream servers from `omni.toml`, keeps
their tools in an in-memory catalog, searches with BM25 (tantivy), validates arguments and
forwards calls. A small benchmark measures Recall@k and token savings. Health checks,
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

- [x] Full system design
- [x] MVP design
- [ ] Design review / approval
- [ ] MVP implementation
- [ ] Benchmark & go/no-go decision

## Tech stack (planned)

Rust · tokio · rmcp · tantivy · jsonschema · serde/toml

## Contributing

Design is still moving. Open an issue to discuss before sending code.
