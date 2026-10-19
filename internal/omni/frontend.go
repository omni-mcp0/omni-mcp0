package omni

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"
)

// Frontend is the MCP server that the AGENT talks to (over stdin/stdout).
// It always exposes exactly four tools, no matter how many tools exist upstream.
type Frontend struct {
	cat   *Catalog
	outMu sync.Mutex // stdout writes must not interleave
}

func NewFrontend(cat *Catalog) *Frontend { return &Frontend{cat: cat} }

// metaTools is the fixed list returned by tools/list. Descriptions are short
// and teach the order: discover -> schema -> invoke.
var metaTools = []map[string]any{
	metaTool("discover_server", "Step 1. Find servers that match an intent.",
		`{"type":"object","properties":{"query":{"type":"string"},"limit":{"type":"integer","default":5}},"required":["query"]}`),
	metaTool("discover_tools", "Step 2. Find tools by intent. Omit 'server' to search all servers.",
		`{"type":"object","properties":{"server":{"type":"string"},"query":{"type":"string"},"limit":{"type":"integer","default":10}},"required":["query"]}`),
	metaTool("get_tool_schema", "Step 3. Get the full input schema of one tool before calling it.",
		`{"type":"object","properties":{"server":{"type":"string"},"tool":{"type":"string"}},"required":["server","tool"]}`),
	metaTool("invoke_tool", "Step 4. Call a tool on a server with arguments matching its schema.",
		`{"type":"object","properties":{"server":{"type":"string"},"tool":{"type":"string"},"arguments":{"type":"object"}},"required":["server","tool"]}`),
}

func metaTool(name, desc, schema string) map[string]any {
	return map[string]any{"name": name, "description": desc, "inputSchema": json.RawMessage(schema)}
}

// Serve reads one JSON-RPC message per line from stdin until EOF.
func (f *Frontend) Serve() {
	slog.Info("serving on stdio")
	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	for sc.Scan() {
		var m rpcMsg
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			slog.Warn("bad json from client", "err", err)
			continue
		}
		// Handle each request in its own goroutine so one slow upstream call
		// does not block the agent's other requests.
		go func() {
			if resp := f.Handle(m); resp != nil {
				f.write(*resp)
			}
		}()
	}
	slog.Info("stdin closed, shutting down")
}

// ServeHTTP lets Omni run as a side server: POST one JSON-RPC message to
// /mcp and get the JSON-RPC reply back. Handy for testing with curl or a
// test script while watching the logs in another terminal.
func (f *Frontend) ServeHTTP(addr string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/mcp", func(w http.ResponseWriter, r *http.Request) {
		var m rpcMsg
		if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
			http.Error(w, "bad json: "+err.Error(), http.StatusBadRequest)
			return
		}
		resp := f.Handle(m)
		if resp == nil { // notification: no reply body
			w.WriteHeader(http.StatusAccepted)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})
	slog.Info("serving on http", "addr", addr, "endpoint", "POST /mcp")
	return http.ListenAndServe(addr, mux)
}

// Handle processes ONE JSON-RPC message and returns the reply
// (nil for notifications). Used by both stdio and HTTP.
func (f *Frontend) Handle(m rpcMsg) (resp *rpcMsg) {
	// A bug while handling one request must never take the whole server down.
	defer func() {
		if r := recover(); r != nil {
			slog.Error("panic while handling request", "method", m.Method, "panic", r)
			resp = fail(m.ID, -32603, fmt.Sprintf("internal error: %v", r))
		}
	}()
	if len(m.ID) == 0 {
		slog.Debug("notification", "method", m.Method)
		return nil // e.g. notifications/initialized: nothing to answer
	}
	switch m.Method {
	case "initialize":
		slog.Info("client connected (initialize)")
		return ok(m.ID, map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "omni-mcp", "version": "0.1.0"},
		})
	case "ping":
		return ok(m.ID, map[string]any{})
	case "tools/list":
		slog.Info("tools/list", "returned", len(metaTools))
		return ok(m.ID, map[string]any{"tools": metaTools})
	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		_ = json.Unmarshal(m.Params, &p)

		start := time.Now()
		slog.Info("-> meta-tool call", "tool", p.Name, "args", truncate(string(p.Arguments), 300))
		res := f.callMetaTool(p.Name, p.Arguments)
		isErr := resultIsError(res)
		slog.Info("<- meta-tool done", "tool", p.Name, "is_error", isErr, "ms", time.Since(start).Milliseconds())
		return ok(m.ID, res)
	default:
		slog.Warn("unknown method", "method", m.Method)
		return fail(m.ID, -32601, "method not found: "+m.Method)
	}
}

// callMetaTool runs one of the four meta-tools. Failures are returned as a
// normal tool result with isError=true (not a protocol error), so the agent
// can read the hint and try again.
func (f *Frontend) callMetaTool(name string, raw json.RawMessage) any {
	var a struct {
		Server    string          `json:"server"`
		Tool      string          `json:"tool"`
		Query     string          `json:"query"`
		Limit     int             `json:"limit"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &a); err != nil {
			return errResult("invalid arguments: " + err.Error())
		}
	}

	switch name {
	case "discover_server":
		hits := f.cat.SearchServers(a.Query, orDefault(a.Limit, 5))
		slog.Info("   search servers", "query", a.Query, "hits", len(hits), "top", topOf(hits))
		return jsonResult(hits)

	case "discover_tools":
		hits := f.cat.SearchTools(a.Server, a.Query, orDefault(a.Limit, 10))
		slog.Info("   search tools", "server", a.Server, "query", a.Query, "hits", len(hits), "top", topOf(hits))
		if len(hits) == 0 {
			return errResult("no tools matched. Try different words, or call discover_server first.")
		}
		return jsonResult(hits)

	case "get_tool_schema":
		_, t, err := f.cat.FindTool(a.Server, a.Tool)
		if err != nil {
			return errResult(err.Error())
		}
		return jsonResult(t)

	case "invoke_tool":
		u, t, err := f.cat.FindTool(a.Server, a.Tool)
		if err != nil {
			return errResult(err.Error())
		}
		if err := checkRequired(t.InputSchema, a.Arguments); err != nil {
			return errResult(err.Error() + ". Call get_tool_schema to see the expected arguments.")
		}
		start := time.Now()
		res, err := u.CallTool(t.Name, a.Arguments)
		slog.Info("   upstream call", "server", a.Server, "tool", t.Name, "ok", err == nil, "ms", time.Since(start).Milliseconds())
		if err != nil {
			return errResult(fmt.Sprintf("server %q failed: %v", a.Server, err))
		}
		return json.RawMessage(res) // upstream result passed through unchanged

	default:
		return errResult("unknown meta-tool " + name)
	}
}

// checkRequired is a tiny argument check: every field listed in the schema's
// "required" must be present. (Full JSON-schema validation comes later.)
func checkRequired(schema, args json.RawMessage) error {
	var s struct {
		Required []string `json:"required"`
	}
	if json.Unmarshal(schema, &s) != nil {
		return nil
	}
	have := map[string]json.RawMessage{}
	_ = json.Unmarshal(args, &have)
	for _, field := range s.Required {
		if _, ok := have[field]; !ok {
			return fmt.Errorf("missing required argument %q", field)
		}
	}
	return nil
}

func orDefault(v, def int) int {
	if v <= 0 {
		return def
	}
	return v
}

// jsonResult wraps any value as MCP text content.
func jsonResult(v any) any {
	b, _ := json.MarshalIndent(v, "", "  ")
	return map[string]any{"content": []any{map[string]any{"type": "text", "text": string(b)}}}
}

func errResult(msg string) any {
	return map[string]any{"isError": true, "content": []any{map[string]any{"type": "text", "text": msg}}}
}

func ok(id json.RawMessage, result any) *rpcMsg {
	b, _ := json.Marshal(result)
	return &rpcMsg{JSONRPC: "2.0", ID: id, Result: b}
}

func fail(id json.RawMessage, code int, msg string) *rpcMsg {
	return &rpcMsg{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: msg}}
}

// truncate keeps log lines short.
func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}

func (f *Frontend) write(m rpcMsg) {
	b, _ := json.Marshal(m)
	f.outMu.Lock()
	defer f.outMu.Unlock()
	os.Stdout.Write(append(b, '\n'))
}

// topOf names the best search hit, for the log.
func topOf(hits []Hit) string {
	if len(hits) == 0 {
		return "-"
	}
	if hits[0].Tool != "" {
		return hits[0].Server + "/" + hits[0].Tool
	}
	return hits[0].Server
}

// resultIsError reports whether a tool result has isError=true. The result may be
// our own map or the upstream's raw JSON, so we look at it as JSON either way.
func resultIsError(res any) bool {
	b, err := json.Marshal(res)
	if err != nil {
		return false
	}
	var r struct {
		IsError bool `json:"isError"`
	}
	_ = json.Unmarshal(b, &r)
	return r.IsError
}
