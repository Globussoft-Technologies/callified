package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"golang.org/x/net/html"
)

var blockedWebsiteNetworks = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"), netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"), netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"), netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"), netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("2001:db8::/32"), netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("64:ff9b:1::/48"), netip.MustParsePrefix("2002::/16"), netip.MustParsePrefix("2001::/32"),
}

func publicWebsiteIP(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	for _, network := range blockedWebsiteNetworks {
		if network.Contains(ip) {
			return false
		}
	}
	return true
}

func websiteURL(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if len(raw) > 2048 {
		return nil, errors.New("Website URL is too long")
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil {
		return nil, errors.New("Enter a public HTTP or HTTPS website URL without embedded credentials")
	}
	if port := u.Port(); port != "" && port != "443" && port != "80" {
		return nil, errors.New("Only standard website ports are allowed")
	}
	if ip, err := netip.ParseAddr(u.Hostname()); err == nil && !publicWebsiteIP(ip) {
		return nil, errors.New("Private or reserved website addresses are not allowed")
	}
	host := strings.ToLower(u.Hostname())
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") {
		return nil, errors.New("Local website addresses are not allowed")
	}
	u.Fragment = ""
	if u.Path == "" {
		u.Path = "/"
	}
	return u, nil
}

func newWebsiteClient() *http.Client {
	transport := &http.Transport{TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 8 * time.Second, MaxIdleConns: 8, IdleConnTimeout: 30 * time.Second}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		addresses, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		if err != nil {
			return nil, fmt.Errorf("Website DNS lookup failed: %w", err)
		}
		if len(addresses) == 0 {
			return nil, errors.New("Website DNS returned no addresses")
		}
		// Validate every answer, then dial the validated IP directly to prevent DNS rebinding.
		for _, ip := range addresses {
			if !publicWebsiteIP(ip) {
				return nil, errors.New("Website resolves to a private or reserved address")
			}
		}
		dialer := net.Dialer{Timeout: 5 * time.Second}
		var lastError error
		for _, ip := range addresses {
			conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
			if err == nil {
				return conn, nil
			}
			lastError = err
		}
		return nil, fmt.Errorf("Could not connect to website: %w", lastError)
	}
	return &http.Client{Transport: transport, Timeout: 15 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("Website redirected too many times")
		}
		_, err := websiteURL(req.URL.String())
		return err
	}}
}

type websitePage struct {
	URL    string   `json:"url"`
	Text   string   `json:"text"`
	Images []string `json:"images"`
}
type websiteCollection struct {
	Pages    []websitePage `json:"pages"`
	Warnings []string      `json:"warnings"`
}

func waitWebsiteRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func retryableWebsiteError(err error) bool {
	var networkError net.Error
	return errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.ECONNREFUSED) || (errors.As(err, &networkError) && (networkError.Timeout() || networkError.Temporary()))
}

func fetchWebsitePage(ctx context.Context, client *http.Client, raw string) ([]byte, string, error) {
	u, err := websiteURL(raw)
	if err != nil {
		return nil, "", err
	}
	for attempt := 0; attempt < 3; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return nil, "", err
		}
		req.Header.Set("User-Agent", "CallifiedProductResearch/1.0")
		req.Header.Set("Accept", "text/html,application/xhtml+xml")
		response, err := client.Do(req)
		delay := time.Duration(attempt+1) * 300 * time.Millisecond
		if err != nil {
			if attempt < 2 && retryableWebsiteError(err) && ctx.Err() == nil {
				if err := waitWebsiteRetry(ctx, delay); err != nil {
					return nil, "", err
				}
				continue
			}
			return nil, "", errors.New("Website connection failed; check the URL, DNS, TLS certificate, or network")
		}
		retry := response.StatusCode == 429 || response.StatusCode == 502 || response.StatusCode == 503 || response.StatusCode == 504
		if retry && attempt < 2 {
			header := response.Header.Get("Retry-After")
			if seconds, e := strconv.Atoi(header); e == nil && seconds >= 0 {
				delay = time.Duration(seconds) * time.Second
			} else if when, e := http.ParseTime(header); e == nil {
				delay = time.Until(when)
			}
			response.Body.Close()
			if delay > 5*time.Second {
				return nil, "", errors.New("Website is rate limited; try again later")
			}
			if delay < 0 {
				delay = 0
			}
			if err := waitWebsiteRetry(ctx, delay); err != nil {
				return nil, "", err
			}
			continue
		}
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			response.Body.Close()
			return nil, "", fmt.Errorf("Website returned HTTP %d; automated access may be unavailable", response.StatusCode)
		}
		kind, _, _ := mime.ParseMediaType(response.Header.Get("Content-Type"))
		if kind != "" && kind != "text/html" && kind != "application/xhtml+xml" {
			response.Body.Close()
			return nil, "", errors.New("The URL did not return an HTML page; use a website page or add the information manually")
		}
		body, err := io.ReadAll(io.LimitReader(response.Body, 1_000_001))
		response.Body.Close()
		if err != nil {
			if attempt < 2 && retryableWebsiteError(err) && ctx.Err() == nil {
				if err := waitWebsiteRetry(ctx, delay); err != nil {
					return nil, "", err
				}
				continue
			}
			return nil, "", errors.New("Website response was interrupted")
		}
		if len(body) > 1_000_000 {
			return nil, "", errors.New("Website page exceeds the supported size")
		}
		if kind == "" && !strings.Contains(http.DetectContentType(body), "text/html") {
			return nil, "", errors.New("Website response is not HTML")
		}
		lower := strings.ToLower(string(body))
		if strings.Contains(lower, "/cdn-cgi/challenge-platform/") || strings.Contains(lower, "<title>just a moment") || strings.Contains(lower, "<title>access denied") || strings.Contains(lower, "<title>verify you are human") {
			return nil, "", errors.New("Website blocks automated access; use approved manual notes or an authorized content export")
		}
		return body, response.Request.URL.String(), nil
	}
	return nil, "", errors.New("Website retries exhausted")
}

func domWebsiteText(raw string) string {
	root, err := html.Parse(strings.NewReader(raw))
	if err != nil {
		return ""
	}
	var out strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "nav", "footer", "aside", "style", "noscript", "svg", "iframe", "canvas", "form":
				return
			case "script":
				for _, a := range n.Attr {
					if a.Key == "type" && a.Val == "application/ld+json" && n.FirstChild != nil && len(n.FirstChild.Data) < 64000 {
						var structured any
						if json.Unmarshal([]byte(n.FirstChild.Data), &structured) == nil {
							encoded, _ := json.Marshal(structured)
							out.WriteString("\nStructured website data: " + string(encoded) + "\n")
						}
					}
				}
				return
			}
			if n.Data == "meta" {
				var name, content string
				for _, a := range n.Attr {
					if a.Key == "name" || a.Key == "property" {
						name = a.Val
					}
					if a.Key == "content" {
						content = a.Val
					}
				}
				if name == "description" || name == "og:description" {
					out.WriteString("\n" + content + "\n")
				}
				return
			}
			for _, a := range n.Attr {
				if a.Key == "hidden" || (a.Key == "aria-hidden" && a.Val == "true") {
					return
				}
			}
		}
		if n.Type == html.TextNode {
			out.WriteString(n.Data)
			out.WriteByte(' ')
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
		if n.Type == html.ElementNode {
			switch n.Data {
			case "p", "div", "section", "h1", "h2", "h3", "h4", "li", "tr", "br", "title":
				out.WriteByte('\n')
			}
		}
	}
	walk(root)
	var lines []string
	for _, line := range strings.Split(out.String(), "\n") {
		line = strings.Join(strings.Fields(line), " ")
		if line != "" {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

func domWebsiteLinks(base, raw string) []string {
	u, err := url.Parse(base)
	if err != nil {
		return nil
	}
	root, err := html.Parse(strings.NewReader(raw))
	if err != nil {
		return nil
	}
	seen := map[string]bool{u.String(): true}
	var result []string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			for _, a := range n.Attr {
				if a.Key == "href" {
					ref, err := url.Parse(a.Val)
					if err != nil {
						continue
					}
					link := u.ResolveReference(ref)
					link.Fragment = ""
					if !strings.EqualFold(link.Hostname(), u.Hostname()) {
						continue
					}
					valid, err := websiteURL(link.String())
					if err != nil || seen[valid.String()] {
						continue
					}
					seen[valid.String()] = true
					result = append(result, valid.String())
				}
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	return result
}

func collectWebsite(ctx context.Context, raw string) (websiteCollection, error) {
	result := websiteCollection{Pages: []websitePage{}, Warnings: []string{}}
	client := newWebsiteClient()
	defer client.CloseIdleConnections()
	body, base, err := fetchWebsitePage(ctx, client, raw)
	if err != nil {
		return result, err
	}
	links := domWebsiteLinks(base, string(body))
	sort.SliceStable(links, func(i, j int) bool { return pagePriority(links[i]) > pagePriority(links[j]) })
	if len(links) > 5 {
		links = links[:5]
	}
	appendPage := func(pageURL string, body []byte) {
		text := domWebsiteText(string(body))
		if len(strings.TrimSpace(text)) < 80 {
			result.Warnings = append(result.Warnings, "A page contained too little readable text: "+pageURL)
			return
		}
		result.Pages = append(result.Pages, websitePage{URL: pageURL, Text: limitWebsiteText(text, 12000), Images: publicImageURLs(extractImageURLs(string(body), pageURL))})
	}
	appendPage(base, body)
	for _, link := range links {
		pageCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
		b, final, err := fetchWebsitePage(pageCtx, client, link)
		cancel()
		if err != nil {
			result.Warnings = append(result.Warnings, link+": "+err.Error())
			continue
		}
		appendPage(final, b)
	}
	if len(result.Pages) == 0 {
		return result, errors.New("No useful website text was found. The site may require JavaScript; add approved manual notes or use an authorized export")
	}
	return result, nil
}

func limitWebsiteText(text string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(text) <= maxBytes {
		return text
	}
	end := maxBytes
	for end > 0 && !utf8.ValidString(text[:end]) {
		end--
	}
	return text[:end]
}

func websitePromptText(pages []websitePage) string {
	if len(pages) == 0 {
		return ""
	}
	budget := 20000 / len(pages)
	var out strings.Builder
	for _, p := range pages {
		out.WriteString("\nSOURCE: " + p.URL + "\n" + limitWebsiteText(p.Text, budget-len(p.URL)-11) + "\n")
	}
	return out.String()
}

func publicImageURLs(images []string) []string {
	var result []string
	for _, raw := range images {
		u, err := websiteURL(raw)
		if err == nil {
			result = append(result, u.String())
		}
		if len(result) == 10 {
			break
		}
	}
	return result
}
