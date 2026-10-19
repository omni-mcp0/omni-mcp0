package omni

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// buildFake compiles cmd/fakeserver into a temp dir and returns its path.
func buildFake(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "fake")
	if out, err := exec.Command("go", "build", "-o", bin, "../../cmd/fakeserver").CombinedOutput(); err != nil {
		t.Fatalf("build fakeserver: %v\n%s", err, out)
	}
	return bin
}

func catalogWithFake(t *testing.T) *Catalog {
	cfg := &Config{Servers: []ServerConfig{
		{Name: "logs", Command: buildFake(t), Description: "Application logs and errors", TimeoutMS: 3000},
		{Name: "broken", Command: "/nonexistent"}, // must be skipped, not fatal
	}}
	return BuildCatalog(cfg)
}

func TestBrokenServerIsSkipped(t *testing.T) {
	c := catalogWithFake(t)
	if len(c.Servers) != 1 || c.Servers["logs"] == nil {
		t.Fatalf("expected only 'logs' connected, got %v", c.Order)
	}
	if n := len(c.Servers["logs"].Tools); n != 2 {
		t.Fatalf("expected 2 tools, got %d", n)
	}
}

func TestSearch(t *testing.T) {
	c := catalogWithFake(t)
	cases := []struct{ query, want string }{
		{"error logs for a service", "query_logs"},
		{"what is the weather in paris", "get_weather"},
		{"query logs", "query_logs"},
	}
	for _, tc := range cases {
		hits := c.SearchTools("", tc.query, 5)
		if len(hits) == 0 || hits[0].Tool != tc.want {
			t.Errorf("query %q: want top hit %q, got %+v", tc.query, tc.want, hits)
		}
	}
	if hits := c.SearchTools("", "zzzz nothing", 5); len(hits) != 0 {
		t.Errorf("expected no hits, got %+v", hits)
	}
	if hits := c.SearchServers("error logs", 5); len(hits) != 1 || hits[0].Server != "logs" {
		t.Errorf("server search failed: %+v", hits)
	}
}

// resultText pulls the first text block out of an MCP tool result.
func resultText(t *testing.T, res any) (string, bool) {
	b, _ := json.Marshal(res)
	var r struct {
		IsError bool `json:"isError"`
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(b, &r); err != nil || len(r.Content) == 0 {
		t.Fatalf("bad result: %s", b)
	}
	return r.Content[0].Text, r.IsError
}

func TestMetaTools(t *testing.T) {
	f := NewFrontend(catalogWithFake(t))
	call := func(name, args string) (string, bool) {
		return resultText(t, f.callMetaTool(name, json.RawMessage(args)))
	}

	if len(metaTools) != 4 {
		t.Fatalf("must expose exactly 4 meta-tools, got %d", len(metaTools))
	}
	if txt, isErr := call("discover_tools", `{"query":"weather"}`); isErr || !strings.Contains(txt, "get_weather") {
		t.Errorf("discover_tools: %v %s", isErr, txt)
	}
	if txt, isErr := call("get_tool_schema", `{"server":"logs","tool":"query_logs"}`); isErr || !strings.Contains(txt, "inputSchema") {
		t.Errorf("get_tool_schema: %v %s", isErr, txt)
	}
	if txt, isErr := call("get_tool_schema", `{"server":"logs","tool":"nope"}`); !isErr || !strings.Contains(txt, "discover_tools") {
		t.Errorf("unknown tool should be isError with hint: %v %s", isErr, txt)
	}
	if txt, isErr := call("invoke_tool", `{"server":"logs","tool":"query_logs","arguments":{}}`); !isErr || !strings.Contains(txt, "service") {
		t.Errorf("missing arg should be isError: %v %s", isErr, txt)
	}
	if txt, isErr := call("invoke_tool", `{"server":"logs","tool":"query_logs","arguments":{"service":"payments"}}`); isErr || !strings.Contains(txt, "called query_logs") {
		t.Errorf("invoke: %v %s", isErr, txt)
	}
	if _, isErr := call("invoke_tool", `{"server":"ghost","tool":"x"}`); !isErr {
		t.Errorf("unknown server should be isError")
	}
}
