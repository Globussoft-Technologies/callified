package api

import (
	"encoding/json"
	"sort"
	"strings"

	"golang.org/x/net/html"
)

// Only public content fields are eligible; application state can also contain
// account details, telemetry, translations, CSS and large inline image data.
func extractCatalogueJSON(raw string) string {
	if len(raw) > maxWebsitePageBytes {
		return ""
	}
	var state map[string]any
	if json.Unmarshal([]byte(raw), &state) != nil {
		return ""
	}
	var out strings.Builder
	nodes := 0
	appendText := func(s string) {
		s = strings.Join(strings.Fields(s), " ")
		if s == "" || strings.Contains(s, "{{") || strings.HasPrefix(s, "data:") || out.Len() >= 12000 {
			return
		}
		out.WriteString(limitWebsiteText(s, 12000-out.Len()))
		out.WriteByte('\n')
	}
	var walk func(any, int)
	walk = func(value any, depth int) {
		nodes++
		if depth > 20 || nodes > 10000 || out.Len() >= 12000 {
			return
		}
		switch v := value.(type) {
		case []any:
			for _, item := range v {
				walk(item, depth+1)
			}
		case map[string]any:
			if hidden, _ := v["isHidden"].(bool); hidden {
				return
			}
			if deleted := v["dateDeleted"]; deleted != nil {
				return
			}
			keys := make([]string, 0, len(v))
			for k := range v {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				switch k {
				case "title", "name", "description", "subtitle", "companyName", "companyAddress", "currencyCode", "price", "sku":
					if s, ok := v[k].(string); ok {
						appendText(k + ": " + s)
					}
					if n, ok := v[k].(float64); ok {
						b, _ := json.Marshal(n)
						appendText(k + ": " + string(b))
					}
				case "sections", "data", "menuItems", "products", "catalogues", "categories":
					walk(v[k], depth+1)
				case "html":
					if s, ok := v[k].(string); ok {
						// Extract text only. Never execute scripts or recursively parse embedded state.
						root, err := html.Parse(strings.NewReader(s))
						if err != nil {
							continue
						}
						var text func(*html.Node)
						text = func(n *html.Node) {
							if n.Type == html.ElementNode {
								switch n.Data {
								case "script", "style", "noscript", "iframe", "svg":
									return
								}
								for _, a := range n.Attr {
									if a.Key == "hidden" || a.Key == "aria-hidden" && a.Val == "true" {
										return
									}
								}
							}
							if n.Type == html.TextNode {
								appendText(n.Data)
							}
							for c := n.FirstChild; c != nil; c = c.NextSibling {
								text(c)
							}
						}
						text(root)
					}
				}
			}
		}
	}
	for _, k := range []string{"companyName", "companyAddress", "currencyCode"} {
		if s, ok := state[k].(string); ok {
			appendText(k + ": " + s)
		}
	}
	for _, k := range []string{"menuItems", "customPage", "products", "catalogues"} {
		walk(state[k], 0)
	}
	return "\n" + limitWebsiteText(out.String(), 12000)
}
