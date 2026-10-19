// A tiny fake upstream MCP server used for local testing.
// It exposes two tools and speaks newline-delimited JSON-RPC on stdio.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
)

func main() {
	sc := bufio.NewScanner(os.Stdin)
	for sc.Scan() {
		var m struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params struct {
				Name string `json:"name"`
			} `json:"params"`
		}
		if json.Unmarshal(sc.Bytes(), &m) != nil || len(m.ID) == 0 {
			continue
		}
		var result string
		switch m.Method {
		case "initialize":
			result = `{"protocolVersion":"2024-11-05","capabilities":{"tools":{}},"serverInfo":{"name":"fake","version":"1"}}`
		case "tools/list":
			// must stay on ONE line: messages are newline-delimited
			result = `{"tools":[{"name":"query_logs","description":"Search error logs for a service","inputSchema":{"type":"object","properties":{"service":{"type":"string"}},"required":["service"]}},{"name":"get_weather","description":"Current weather for a city","inputSchema":{"type":"object"}}]}`
		case "tools/call":
			result = fmt.Sprintf(`{"content":[{"type":"text","text":"called %s"}]}`, m.Params.Name)
		default:
			result = `{}`
		}
		fmt.Printf(`{"jsonrpc":"2.0","id":%s,"result":%s}`+"\n", m.ID, result)
	}
}
