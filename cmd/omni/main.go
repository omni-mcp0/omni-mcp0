// Omni MCP: one MCP server that fronts many upstream MCP servers.
// Usage: omni [-config omni.json]
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/omni-mcp0/omni-mcp0/internal/omni"
)

func main() {
	path := flag.String("config", "omni.json", "path to config file")
	flag.Parse()

	cfg, err := omni.LoadConfig(*path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "omni:", err)
		os.Exit(1)
	}

	// IMPORTANT: stdout is the MCP channel, so all logs go to stderr.
	cat := omni.BuildCatalog(cfg)
	omni.NewFrontend(cat).Serve() // blocks until the agent disconnects
}
