// Omni MCP: one MCP server that fronts many upstream MCP servers.
//
// Usage:
//
//	omni -config omni.json                 # stdio (for Claude Code / Cursor)
//	omni -config omni.json -listen :8080   # HTTP side server (for testing + logs)
//	omni -log-file omni.log -debug         # also write logs to a file, verbose
package main

import (
	"flag"
	"io"
	"log/slog"
	"os"

	"github.com/omni-mcp0/omni-mcp0/internal/omni"
)

func main() {
	path := flag.String("config", "omni.json", "path to config file")
	listen := flag.String("listen", "", "serve HTTP on this address (e.g. :8080) instead of stdio")
	logFile := flag.String("log-file", "", "also append logs to this file")
	debug := flag.Bool("debug", false, "verbose logging")
	flag.Parse()

	// IMPORTANT: in stdio mode stdout is the MCP channel, so logs go to stderr.
	var out io.Writer = os.Stderr
	if *logFile != "" {
		f, err := os.OpenFile(*logFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			slog.Error("cannot open log file", "err", err)
			os.Exit(1)
		}
		defer f.Close()
		out = io.MultiWriter(os.Stderr, f)
	}
	level := slog.LevelInfo
	if *debug {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(out, &slog.HandlerOptions{Level: level})))

	cfg, err := omni.LoadConfig(*path)
	if err != nil {
		slog.Error("cannot load config", "err", err)
		os.Exit(1)
	}

	cat := omni.BuildCatalog(cfg)
	fe := omni.NewFrontend(cat)

	if *listen != "" {
		if err := fe.ServeHTTP(*listen); err != nil {
			slog.Error("http server stopped", "err", err)
			os.Exit(1)
		}
		return
	}
	fe.Serve() // blocks until the agent disconnects
}
