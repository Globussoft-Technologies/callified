package api

import (
	"encoding/json"
	"strings"
)

// Quicksell storefronts render their published page from amalgam-json. Decode
// only presentation fields: never send visitor identities, tracking settings,
// inventory internals, executable scripts, or the full hydration blob to AI.
func catalogueWebsiteText(raw string) string {
	type item struct {
		Name        string          `json:"name"`
		Title       string          `json:"title"`
		Description string          `json:"description"`
		Hidden      bool            `json:"isHidden"`
		Draft       bool            `json:"draft"`
		Deleted     json.RawMessage `json:"timestamp_deleted"`
	}
	var state struct {
		CompanyName string `json:"companyName"`
		CustomPage  struct {
			Published bool   `json:"isPublished"`
			Title     string `json:"title"`
			Sections  []struct {
				Title  string            `json:"title"`
				Hidden bool              `json:"isHidden"`
				Data   []json.RawMessage `json:"data"`
			} `json:"sections"`
			Catalogues []item `json:"catalogues"`
			Products   []item `json:"products"`
		} `json:"customPage"`
	}
	if len(raw) > maxWebsitePageBytes || json.Unmarshal([]byte(raw), &state) != nil || !state.CustomPage.Published {
		return ""
	}
	var out strings.Builder
	appendText := func(text string) {
		if remaining := 12000 - out.Len() - 1; remaining > 0 {
			out.WriteString(limitWebsiteText(strings.TrimSpace(text), remaining))
			out.WriteByte('\n')
		}
	}
	appendText(state.CompanyName)
	appendText(state.CustomPage.Title)
	for i, section := range state.CustomPage.Sections {
		if i >= 20 || out.Len() >= 12000 {
			break
		}
		if section.Hidden {
			continue
		}
		appendText(section.Title)
		for _, rawBlock := range section.Data {
			if out.Len() >= 12000 {
				break
			}
			// Catalogue/product sections contain IDs; HTML sections contain
			// objects. Ignore references rather than rejecting the whole page.
			var block struct {
				HTML string `json:"html"`
			}
			if json.Unmarshal(rawBlock, &block) != nil {
				continue
			}
			appendText(domWebsiteTextContent(block.HTML, false))
		}
	}
	appendItems := func(items []item) {
		for i, entry := range items {
			if i >= 200 || out.Len() >= 12000 {
				break
			}
			deleted := strings.TrimSpace(string(entry.Deleted))
			if entry.Hidden || entry.Draft || (deleted != "" && deleted != "null" && deleted != "0" && deleted != `""`) {
				continue
			}
			appendText(entry.Title)
			appendText(entry.Name)
			appendText(domWebsiteTextContent(entry.Description, false))
		}
	}
	appendItems(state.CustomPage.Catalogues)
	appendItems(state.CustomPage.Products)
	return strings.TrimSpace(out.String())
}
