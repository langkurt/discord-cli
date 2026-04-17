package discord

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// supportedDomains are hosts we know how to extract images from.
var supportedDomains = []string{
	"fxtwitter.com",
	"fixupx.com",
	"vxtwitter.com",
	"fixvx.com",
	"phixiv.net",
	"pixiv.net",
	"artstation.com",
	"imgur.com",
	"i.imgur.com",
}

// ogImageRe matches <meta property="og:image" content="..."> in any attribute order.
var ogImageRe = regexp.MustCompile(`(?i)<meta[^>]+property=["']og:image["'][^>]+content=["']([^"']+)["']|<meta[^>]+content=["']([^"']+)["'][^>]+property=["']og:image["']`)

// urlRe extracts http(s) URLs from message content.
var urlRe = regexp.MustCompile(`https?://[^\s<>"]+`)

// perDomainDelay controls politeness per host.
var perDomainDelay = map[string]time.Duration{
	"phixiv.net": 1 * time.Second,
	"pixiv.net":  1 * time.Second,
	"default":    400 * time.Millisecond,
}

// IsSupportedLinkDomain returns true if we know how to extract an image from this URL.
func IsSupportedLinkDomain(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	for _, d := range supportedDomains {
		if host == d || strings.HasSuffix(host, "."+d) {
			return true
		}
	}
	return false
}

// ExtractURLs returns all http(s) URLs found in a string that are from supported domains.
func ExtractURLs(content string) []string {
	all := urlRe.FindAllString(content, -1)
	var filtered []string
	for _, u := range all {
		if IsSupportedLinkDomain(u) {
			filtered = append(filtered, u)
		}
	}
	return filtered
}

// FetchOGImage fetches a page and returns the og:image URL.
func FetchOGImage(client *http.Client, pageURL string) (string, error) {
	req, err := http.NewRequest("GET", pageURL, nil)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	// Mimic a real browser so sites don't block us
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status %d", resp.StatusCode)
	}

	// Read up to 128KB — og:image is always in <head>, no need for full body
	body, err := io.ReadAll(io.LimitReader(resp.Body, 128*1024))
	if err != nil {
		return "", fmt.Errorf("read body: %w", err)
	}

	m := ogImageRe.FindSubmatch(body)
	if m == nil {
		return "", fmt.Errorf("no og:image found")
	}
	// One of the two capture groups will be non-empty
	imgURL := string(m[1])
	if imgURL == "" {
		imgURL = string(m[2])
	}
	imgURL = strings.TrimSpace(imgURL)
	if imgURL == "" {
		return "", fmt.Errorf("empty og:image")
	}
	return imgURL, nil
}

// DelayForURL returns the appropriate polite delay for a given URL's domain.
func DelayForURL(rawURL string) time.Duration {
	u, err := url.Parse(rawURL)
	if err != nil {
		return perDomainDelay["default"]
	}
	host := strings.ToLower(u.Hostname())
	for domain, delay := range perDomainDelay {
		if domain == "default" {
			continue
		}
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return delay
		}
	}
	return perDomainDelay["default"]
}
