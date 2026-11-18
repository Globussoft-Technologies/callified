package api

import (
	"context"
	"io"
	"net/http"
	"net/netip"
	"strings"
	"syscall"
	"testing"
	"time"
	"unicode/utf8"
)

type websiteRoundTrip func(*http.Request) (*http.Response, error)

func (f websiteRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func mockWebsiteResponse(r *http.Request, status int, kind, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{kind}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}
}

func TestPublicWebsiteDestinations(t *testing.T) {
	for _, raw := range []string{"127.0.0.1", "10.0.0.1", "172.16.1.1", "192.168.1.1", "169.254.169.254", "100.64.1.1", "0.0.0.0", "192.0.2.1", "198.18.0.1", "::1", "fc00::1", "fe80::1", "::ffff:127.0.0.1", "2002:7f00:1::1", "64:ff9b::7f00:1", "2001:db8::1"} {
		if publicWebsiteIP(netip.MustParseAddr(raw)) {
			t.Errorf("allowed private/reserved %s", raw)
		}
	}
	for _, raw := range []string{"8.8.8.8", "1.1.1.1", "2606:4700:4700::1111"} {
		if !publicWebsiteIP(netip.MustParseAddr(raw)) {
			t.Errorf("blocked public %s", raw)
		}
	}
	for _, raw := range []string{"http://localhost/a", "http://foo.local/", "http://127.0.0.1/", "http://[::1]/", "ftp://example.com", "https://name:password@example.com", "https://example.com:8001"} {
		if _, err := websiteURL(raw); err == nil {
			t.Errorf("allowed unsafe URL %s", raw)
		}
	}
	u, err := websiteURL(" example.com/path#section ")
	if err != nil || u.String() != "https://example.com/path" {
		t.Fatalf("normalization: %v %v", u, err)
	}
}

func TestWebsiteRedirectBlocksPrivateAddress(t *testing.T) {
	client := newWebsiteClient()
	defer client.CloseIdleConnections()
	calls := 0
	client.Transport = websiteRoundTrip(func(r *http.Request) (*http.Response, error) {
		calls++
		response := mockWebsiteResponse(r, 302, "text/html", "")
		response.Header.Set("Location", "http://169.254.169.254/latest/meta-data")
		return response, nil
	})
	_, _, err := fetchWebsitePage(context.Background(), client, "https://example.com")
	if err == nil || calls != 1 {
		t.Fatalf("redirect must be rejected before second request: %v calls %d", err, calls)
	}
}

func TestWebsiteHTTPValidation(t *testing.T) {
	cases := []struct {
		name       string
		status     int
		kind, body string
	}{
		{"access denied", 403, "text/html", "<p>Not authorized</p>"},
		{"non html", 200, "application/json", "{}"},
		{"challenge", 200, "text/html", "<title>Just a moment...</title><script src='/cdn-cgi/challenge-platform/x'></script>"},
		{"oversize", 200, "text/html", strings.Repeat("x", maxWebsitePageBytes+1)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			client := &http.Client{Transport: websiteRoundTrip(func(r *http.Request) (*http.Response, error) {
				calls++
				return mockWebsiteResponse(r, tc.status, tc.kind, tc.body), nil
			})}
			_, _, err := fetchWebsitePage(context.Background(), client, "https://example.com")
			if err == nil || calls != 1 {
				t.Fatalf("expected non-retryable failure, got %v calls %d", err, calls)
			}
		})
	}
}

func TestWebsiteResearchAcceptsLargeStorefrontAndKeepsTextBudgets(t *testing.T) {
	// Product content follows the large script block: reading only the old 1 MB
	// limit would lose it, and passing script data to the model would add noise.
	raw := "<html><head><script>" + strings.Repeat("application_data;", 150000) +
		"</script></head><body><h1>Sanitary products</h1><p>" + strings.Repeat("Basins and bathroom fittings. ", 1000) + "</p></body></html>"
	client := &http.Client{Transport: websiteRoundTrip(func(r *http.Request) (*http.Response, error) {
		return mockWebsiteResponse(r, 200, "text/html", raw), nil
	})}
	body, final, err := fetchWebsitePage(context.Background(), client, "https://example.com")
	if err != nil || len(body) != len(raw) {
		t.Fatalf("large storefront rejected or truncated: bytes=%d err=%v", len(body), err)
	}
	text := domWebsiteText(string(body))
	if !strings.Contains(text, "Sanitary products") || strings.Contains(text, "application_data") {
		t.Fatal("product content lost or scripts included in research text")
	}
	page := websitePage{URL: final, Text: limitWebsiteText(text, 12000)}
	if len(page.Text) > 12000 || len(websitePromptText([]websitePage{page})) > 20000 {
		t.Fatal("large HTML bypassed research text budgets")
	}
}

func TestWebsiteHTMLSizeLimitIncludesBoundary(t *testing.T) {
	for _, size := range []int{maxWebsitePageBytes, maxWebsitePageBytes + 1} {
		client := &http.Client{Transport: websiteRoundTrip(func(r *http.Request) (*http.Response, error) {
			return mockWebsiteResponse(r, 200, "text/html", strings.Repeat(" ", size)), nil
		})}
		body, _, err := fetchWebsitePage(context.Background(), client, "https://example.com")
		if size == maxWebsitePageBytes && (err != nil || len(body) != size) {
			t.Fatalf("size limit boundary must be accepted: %v", err)
		}
		if size > maxWebsitePageBytes && (err == nil || body != nil || !strings.Contains(err.Error(), "8 MB")) {
			t.Fatalf("oversized HTML must be rejected with an actionable error: %v", err)
		}
	}
}

func TestWebsiteRetriesAndCancellation(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: websiteRoundTrip(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls < 3 {
			resp := mockWebsiteResponse(r, 503, "text/html", "")
			resp.Header.Set("Retry-After", "0")
			return resp, nil
		}
		return mockWebsiteResponse(r, 200, "text/html", "<p>Approved product details</p>"), nil
	})}
	body, _, err := fetchWebsitePage(context.Background(), client, "https://example.com")
	if err != nil || calls != 3 || len(body) == 0 {
		t.Fatalf("retry: %v calls %d", err, calls)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := waitWebsiteRetry(ctx, time.Second); err == nil {
		t.Fatal("retry must respect cancellation")
	}
}

func TestWebsiteTransientConnectionRetry(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: websiteRoundTrip(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return nil, syscall.ECONNRESET
		}
		return mockWebsiteResponse(r, 200, "text/html", "<p>Useful website</p>"), nil
	})}
	_, _, err := fetchWebsitePage(context.Background(), client, "https://example.com")
	if err != nil || calls != 2 {
		t.Fatalf("transient retry: %v calls %d", err, calls)
	}
}

func TestDOMWebsiteExtraction(t *testing.T) {
	raw := `<html><head><meta name='description' content='Product summary'><script type='application/ld+json'>{"@type":"Product","offers":{"price":100}}</script></head><body><nav>Ignore navigation</nav><h1>Useful service</h1><p>Fast &amp; reliable</p><script>do not extract this</script><footer>Ignore footer</footer><p hidden>Secret hidden</p><a href = '/pricing?plan=basic'>Pricing</a><a href='http://127.0.0.1/'>Private</a></body></html>`
	text := domWebsiteText(raw)
	for _, wanted := range []string{"Product summary", "Useful service", "Fast & reliable", "\"price\":100"} {
		if !strings.Contains(text, wanted) {
			t.Errorf("missing %q in %s", wanted, text)
		}
	}
	for _, bad := range []string{"Ignore navigation", "Ignore footer", "do not extract", "Secret hidden"} {
		if strings.Contains(text, bad) {
			t.Errorf("included noise %q", bad)
		}
	}
	links := domWebsiteLinks("https://example.com/", raw)
	if len(links) != 1 || links[0] != "https://example.com/pricing?plan=basic" {
		t.Fatalf("links %v", links)
	}
}

func TestWebsiteBalancedBudget(t *testing.T) {
	pages := []websitePage{{URL: "https://example.com/", Text: strings.Repeat("Homepage ", 9000)}, {URL: "https://example.com/pricing", Text: "PRICE IS 100\n" + strings.Repeat("नमस्ते", 9000)}}
	text := websitePromptText(pages)
	if !utf8.ValidString(text) || len(text) > 20000 || !strings.Contains(text, "PRICE IS 100") {
		t.Fatalf("bad budget: %d", len(text))
	}
	if got := limitWebsiteText("नमस्ते", 5); !utf8.ValidString(got) {
		t.Fatal("split UTF8")
	}
}
