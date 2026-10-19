package omni

import (
	"math"
	"sort"
	"strings"
	"unicode"
)

// Hit is one search result.
type Hit struct {
	Server      string  `json:"server"`
	Tool        string  `json:"tool,omitempty"`
	Description string  `json:"description"`
	ToolCount   int     `json:"tool_count,omitempty"`
	Score       float64 `json:"score"`
}

// doc is one searchable item (a tool, or a server).
type doc struct {
	hit    Hit
	tokens map[string]float64 // word -> weighted count
}

// tokenize lowercases and splits text into words.
// "get_pull_request" -> [get pull request], so snake_case names are searchable.
func tokenize(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

// addText adds words from text into the doc with a weight (name matters more than description).
func (d *doc) addText(text string, weight float64) {
	for _, w := range tokenize(text) {
		d.tokens[w] += weight
	}
}

// rank scores docs against the query using weighted term frequency x IDF
// (a simple cousin of BM25: rare words count more than common words).
func rank(docs []*doc, query string, limit int) []Hit {
	words := tokenize(query)
	if len(words) == 0 || len(docs) == 0 {
		return nil
	}
	// how many docs contain each word (for IDF)
	df := map[string]int{}
	for _, d := range docs {
		for w := range d.tokens {
			df[w]++
		}
	}
	n := float64(len(docs))
	var hits []Hit
	for _, d := range docs {
		score := 0.0
		for _, w := range words {
			tf := d.tokens[w]
			if tf == 0 {
				continue
			}
			idf := math.Log(1 + (n-float64(df[w])+0.5)/(float64(df[w])+0.5))
			score += idf * tf / (tf + 1.2) // saturating tf, like BM25
		}
		if score > 0 {
			h := d.hit
			h.Score = math.Round(score*100) / 100
			hits = append(hits, h)
		}
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].Score > hits[j].Score })
	if limit > 0 && len(hits) > limit {
		hits = hits[:limit]
	}
	return hits
}

// SearchServers ranks servers for a query (name, description, tags, plus their tool names).
func (c *Catalog) SearchServers(query string, limit int) []Hit {
	var docs []*doc
	for _, name := range c.Order {
		u := c.Servers[name]
		d := &doc{hit: Hit{Server: name, Description: u.Cfg.Description, ToolCount: len(u.Tools)}, tokens: map[string]float64{}}
		d.addText(name, 3)
		d.addText(u.Cfg.Description, 2)
		d.addText(strings.Join(u.Cfg.Tags, " "), 2)
		for _, t := range u.Tools {
			d.addText(t.Name, 1) // a server matches if any of its tools match
		}
		docs = append(docs, d)
	}
	return rank(docs, query, limit)
}

// SearchTools ranks tools for a query. If server is "" it searches everything.
func (c *Catalog) SearchTools(server, query string, limit int) []Hit {
	var docs []*doc
	for _, name := range c.Order {
		if server != "" && name != server {
			continue
		}
		u := c.Servers[name]
		for _, t := range u.Tools {
			d := &doc{hit: Hit{Server: name, Tool: t.Name, Description: firstLine(t.Description)}, tokens: map[string]float64{}}
			d.addText(t.Name, 3)        // tool name: strongest signal
			d.addText(t.Description, 2) // description
			d.addText(name, 1)          // server name helps a bit
			docs = append(docs, d)
		}
	}
	return rank(docs, query, limit)
}

// firstLine keeps search results short: first line, max 200 chars.
func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 200 {
		s = s[:200] + "..."
	}
	return s
}
