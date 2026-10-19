package omni

import (
	"fmt"
	"os"
)

// Catalog is the in-memory list of all connected servers and their tools.
// MVP: built once at startup, never changes afterwards.
type Catalog struct {
	Servers map[string]*Upstream
	Order   []string // server names in config order (stable output)
}

// BuildCatalog connects to every configured server. A server that fails to
// start is skipped (logged), so one broken server never breaks the rest.
func BuildCatalog(cfg *Config) *Catalog {
	c := &Catalog{Servers: map[string]*Upstream{}}
	for _, sc := range cfg.Servers {
		u, err := Connect(sc)
		if err != nil {
			fmt.Fprintf(os.Stderr, "omni: skipping server %q: %v\n", sc.Name, err)
			continue
		}
		c.Servers[sc.Name] = u
		c.Order = append(c.Order, sc.Name)
		fmt.Fprintf(os.Stderr, "omni: connected %q (%d tools)\n", sc.Name, len(u.Tools))
	}
	return c
}

// FindTool looks up a tool by the (server, tool) pair.
func (c *Catalog) FindTool(server, tool string) (*Upstream, *Tool, error) {
	u, ok := c.Servers[server]
	if !ok {
		return nil, nil, fmt.Errorf("unknown server %q. Use discover_server to list servers", server)
	}
	for i := range u.Tools {
		if u.Tools[i].Name == tool {
			return u, &u.Tools[i], nil
		}
	}
	return nil, nil, fmt.Errorf("unknown tool %q on server %q. Use discover_tools to search", tool, server)
}
