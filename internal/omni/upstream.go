package omni

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"
)

// Tool is one tool exposed by an upstream server (fields follow the MCP spec).
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
	Annotations json.RawMessage `json:"annotations,omitempty"`
}

// rpcMsg is a JSON-RPC 2.0 message. Used for both requests and responses.
type rpcMsg struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Upstream is a connection to ONE upstream MCP server (a child process).
// Each upstream has its own process and goroutines, so a slow or crashing
// server cannot block the others.
type Upstream struct {
	Cfg   ServerConfig
	Tools []Tool // filled by Connect

	stdin   *bufio.Writer
	writeMu sync.Mutex // one writer at a time

	mu      sync.Mutex // protects nextID and pending
	nextID  int
	pending map[int]chan rpcMsg // request id -> channel waiting for the reply
}

// Connect starts the child process, runs the MCP handshake and loads its tools.
func Connect(cfg ServerConfig) (*Upstream, error) {
	cmd := exec.Command(cfg.Command, cfg.Args...)
	cmd.Env = os.Environ()
	for k, v := range cfg.Env {
		cmd.Env = append(cmd.Env, k+"="+os.ExpandEnv(v)) // supports "${TOKEN}" in config
	}
	cmd.Stderr = os.Stderr // upstream logs go to our stderr (stdout is reserved for MCP)

	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %q: %w", cfg.Command, err)
	}

	u := &Upstream{Cfg: cfg, stdin: bufio.NewWriter(in), pending: map[int]chan rpcMsg{}}
	go u.readLoop(out) // routes replies to whoever is waiting

	// 1. initialize handshake
	initParams := map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "omni-mcp", "version": "0.1.0"},
	}
	if _, err := u.call("initialize", initParams); err != nil {
		return nil, fmt.Errorf("initialize: %w", err)
	}
	u.notify("notifications/initialized")

	// 2. load every tool (follow pagination via nextCursor)
	cursor := ""
	for {
		params := map[string]any{}
		if cursor != "" {
			params["cursor"] = cursor
		}
		raw, err := u.call("tools/list", params)
		if err != nil {
			return nil, fmt.Errorf("tools/list: %w", err)
		}
		var page struct {
			Tools      []Tool `json:"tools"`
			NextCursor string `json:"nextCursor"`
		}
		if err := json.Unmarshal(raw, &page); err != nil {
			return nil, err
		}
		u.Tools = append(u.Tools, page.Tools...)
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	return u, nil
}

// CallTool forwards a tools/call to the upstream and returns its raw result.
func (u *Upstream) CallTool(name string, args json.RawMessage) (json.RawMessage, error) {
	var arguments any = map[string]any{}
	if len(args) > 0 {
		arguments = args
	}
	return u.call("tools/call", map[string]any{"name": name, "arguments": arguments})
}

// call sends a request and waits for the matching reply, with a timeout.
func (u *Upstream) call(method string, params any) (json.RawMessage, error) {
	u.mu.Lock()
	u.nextID++
	id := u.nextID
	ch := make(chan rpcMsg, 1)
	u.pending[id] = ch
	u.mu.Unlock()

	p, _ := json.Marshal(params)
	if err := u.send(rpcMsg{JSONRPC: "2.0", ID: json.RawMessage(fmt.Sprint(id)), Method: method, Params: p}); err != nil {
		return nil, err
	}

	timeout := time.Duration(u.Cfg.TimeoutMS) * time.Millisecond
	if timeout == 0 {
		timeout = 15 * time.Second
	}
	select {
	case reply, ok := <-ch:
		if !ok {
			return nil, fmt.Errorf("server %q exited", u.Cfg.Name)
		}
		if reply.Error != nil {
			return nil, fmt.Errorf("%s", reply.Error.Message)
		}
		return reply.Result, nil
	case <-time.After(timeout):
		u.mu.Lock()
		delete(u.pending, id)
		u.mu.Unlock()
		return nil, fmt.Errorf("timeout after %s", timeout)
	}
}

// notify sends a message that expects no reply.
func (u *Upstream) notify(method string) {
	_ = u.send(rpcMsg{JSONRPC: "2.0", Method: method})
}

func (u *Upstream) send(m rpcMsg) error {
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	u.writeMu.Lock()
	defer u.writeMu.Unlock()
	if _, err := u.stdin.Write(append(b, '\n')); err != nil {
		return err
	}
	return u.stdin.Flush()
}

// readLoop reads newline-delimited JSON from the child and hands each reply
// to the goroutine waiting on that id. When the child exits, waiting callers
// are released with an error.
func (u *Upstream) readLoop(r interface{ Read([]byte) (int, error) }) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1024*1024), 16*1024*1024) // allow big messages
	for sc.Scan() {
		var m rpcMsg
		if json.Unmarshal(sc.Bytes(), &m) != nil || m.Method != "" || len(m.ID) == 0 {
			continue // ignore garbage, notifications and server-initiated requests
		}
		var id int
		if json.Unmarshal(m.ID, &id) != nil {
			continue
		}
		u.mu.Lock()
		ch := u.pending[id]
		delete(u.pending, id)
		u.mu.Unlock()
		if ch != nil {
			ch <- m
		}
	}
	// process ended: wake everyone still waiting
	u.mu.Lock()
	for id, ch := range u.pending {
		close(ch)
		delete(u.pending, id)
	}
	u.mu.Unlock()
}
