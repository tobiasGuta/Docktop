// Package mcpcatalog parses Docker MCP's human-readable catalog list output.
// The format isn't a stable API; unknown fields are ignored and raw error text
// is retained by the UI when parsing is unsuccessful.
package mcpcatalog

import (
	"strconv"
	"strings"
)

type Server struct {
	Name, Title, Description, Type, Image, Endpoint string
	Tools                                           int
}

// Parse expects entries indented by two spaces and properties by four.
// The server cap prevents huge catalog outputs from exhausting the TUI.
func Parse(raw string) []Server {
	var servers []Server
	var current *Server
	flush := func() {
		if current != nil && current.Name != "" && len(servers) < 1000 {
			servers = append(servers, *current)
		}
	}
	for _, line := range strings.Split(raw, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		switch {
		case strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "    ") && strings.TrimSpace(line) != "":
			flush()
			current = &Server{Name: strings.TrimSpace(line)}
		case strings.HasPrefix(line, "    ") && current != nil:
			key, value, ok := strings.Cut(strings.TrimSpace(line), ":")
			if !ok {
				continue
			}
			value = strings.TrimSpace(value)
			switch key {
			case "Title":
				current.Title = value
			case "Description":
				current.Description = value
			case "Type":
				current.Type = value
			case "Image":
				current.Image = value
			case "Endpoint":
				current.Endpoint = value
			case "Tools":
				current.Tools, _ = strconv.Atoi(value)
			}
		}
	}
	flush()
	return servers
}

func Filter(servers []Server, query string) []Server {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return servers
	}
	out := make([]Server, 0, len(servers))
	for _, s := range servers {
		if strings.Contains(strings.ToLower(s.Name+" "+s.Title+" "+s.Description), query) {
			out = append(out, s)
		}
	}
	return out
}
