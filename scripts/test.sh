#!/usr/bin/env bash
# Sends a few requests to a running Omni side server (see README: "Watch the logs").
# Usage: scripts/test.sh [http://localhost:8080/mcp]
URL="${1:-http://localhost:8080/mcp}"

call() { # call <title> <json>
  echo "=== $1"
  curl -s -X POST "$URL" -H 'Content-Type: application/json' -d "$2" | head -c 600
  echo; echo
}

call "initialize" '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}'
call "tools/list (must be exactly 4 tools)" '{"jsonrpc":"2.0","id":2,"method":"tools/list"}'
call "discover_server" '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"discover_server","arguments":{"query":"read a file"}}}'
call "discover_tools" '{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"discover_tools","arguments":{"query":"search files by pattern","limit":3}}}'
call "get_tool_schema" '{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"get_tool_schema","arguments":{"server":"filesystem","tool":"list_directory"}}}'
call "invoke_tool (missing argument -> isError)" '{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"invoke_tool","arguments":{"server":"filesystem","tool":"list_directory","arguments":{}}}}'
call "invoke_tool (ok)" '{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"invoke_tool","arguments":{"server":"filesystem","tool":"list_directory","arguments":{"path":"/tmp"}}}}'
