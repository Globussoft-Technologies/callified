package api

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestEmbeddedCatalogueContent(t *testing.T) {
	raw := `<script type="application/json" id="amalgam-json">{"companyName":"United Sanitary","userEmail":"private@example.com","i18n":{"title":"Translation noise"},"menuItems":[{"title":"PIPES & FITTINGS","menuItems":[{"title":"Supreme"}]},{"title":"Hidden category","isHidden":true}],"customPage":{"sections":[{"title":"Sanitaryware","data":[{"html":"<p>Cera basins</p>\u003cscript>evil()\u003c/script><img src='data:image/png;base64,SECRETIMAGE'>"}]}]}}</script>`
	got := domWebsiteText(raw)
	for _, want := range []string{"United Sanitary", "PIPES & FITTINGS", "Supreme", "Cera basins"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s in %s", want, got)
		}
	}
	for _, excluded := range []string{"private@example.com", "Translation noise", "Hidden category", "evil()", "SECRETIMAGE"} {
		if strings.Contains(got, excluded) {
			t.Errorf("included %s", excluded)
		}
	}
	if extractCatalogueJSON(`{broken`) != "" {
		t.Fatal("accepted malformed JSON")
	}
	got = extractCatalogueJSON(`{"companyName":"` + strings.Repeat("a", 30000) + `"}`)
	if len(got) > 12001 {
		t.Fatalf("unbounded text: %d", len(got))
	}
}

func TestWebsiteDownloadAboveOldLimit(t *testing.T) {
	body := "<html><body>" + strings.Repeat("x", 2296359) + "</body></html>"
	client := &http.Client{Transport: websiteRoundTrip(func(r *http.Request) (*http.Response, error) {
		return mockWebsiteResponse(r, 200, "text/html", body), nil
	})}
	got, _, err := fetchWebsitePage(context.Background(), client, "https://example.com")
	if err != nil || string(got) != body {
		t.Fatalf("large valid page failed: %v", err)
	}
}
