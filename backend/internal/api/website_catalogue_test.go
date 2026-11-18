package api

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestPublishedCatalogueResearchOnlyExtractsPresentationFields(t *testing.T) {
	state := map[string]any{
		"companyName":        "Example Sanitary",
		"userEmail":          "visitor-secret@example.com",
		"visitorId":          "tracking-secret",
		"companyExperiments": map[string]any{"internal": "experiment-secret"},
		"customPage": map[string]any{
			"isPublished": true, "title": "Bathroom collection",
			"sections": []any{
				map[string]any{"title": "Our catalogues", "data": []any{"catalogue-reference"}},
				map[string]any{"title": "About us", "data": []any{map[string]any{"html": "<p>Bathroom fittings since 2000.</p><script>executable-secret</script>", "script": "section-script-secret"}}},
				map[string]any{"title": "hidden-section-secret", "isHidden": true},
			},
			"catalogues": []any{
				map[string]any{"title": "Basins", "description": "Ceramic fixtures"},
				map[string]any{"title": "hidden-secret", "isHidden": true},
				map[string]any{"title": "draft-secret", "draft": true},
				map[string]any{"title": "deleted-secret", "timestamp_deleted": "12345"},
			},
			"products": []any{map[string]any{"name": "Wall basin", "description": "<p>Compact design</p>", "stock": "inventory-secret", "price": "unverified-price-secret"}},
		},
	}
	encoded, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	raw := `<script type="application/json" id="amalgam-json">` + string(encoded) + `</script>`
	text := domWebsiteText(raw)
	for _, expected := range []string{"Example Sanitary", "Bathroom collection", "Bathroom fittings since 2000", "Basins", "Wall basin", "Compact design"} {
		if !strings.Contains(text, expected) {
			t.Errorf("missing public content %q", expected)
		}
	}
	if strings.Contains(text, "secret") {
		t.Fatalf("non-presentation data leaked: %s", text)
	}
	if text := domWebsiteText(strings.Replace(raw, "amalgam-json", "unknown-state", 1)); text != "" {
		t.Fatal("unknown JSON hydration was extracted")
	}
}

func TestCatalogueResearchSkipsUnpublishedAndBoundsText(t *testing.T) {
	if text := catalogueWebsiteText(`{"companyName":"Secret","customPage":{"isPublished":false,"title":"Draft page"}}`); text != "" {
		t.Fatal("unpublished page was extracted")
	}
	if text := catalogueWebsiteText(`not json`); text != "" {
		t.Fatal("malformed state was accepted")
	}
	state := map[string]any{"customPage": map[string]any{"isPublished": true, "products": []any{map[string]any{"name": strings.Repeat("नमस्ते", 3000)}}}}
	encoded, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	text := catalogueWebsiteText(string(encoded))
	if len(text) > 12000 || !utf8.ValidString(text) {
		t.Fatal("catalogue exceeded safe text budget or split UTF-8")
	}
}
