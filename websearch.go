package main

// Web tools for the assistant: DuckDuckGo search (no API key) and a safe page reader.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"

	"golang.org/x/net/html"
)

const webUA = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130 Safari/537.36"

type webResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

// publicOnly refuses connections to loopback, private, link-local and other non-public
// addresses, so the AI can't be used to reach services running on the server or LAN.
func publicOnly(network, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	ip := net.ParseIP(host)
	if ip == nil || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified() || ip.IsMulticast() || ip.IsInterfaceLocalMulticast() ||
		(ip.To4() != nil && ip.To4()[0] == 100 && ip.To4()[1]&0xc0 == 64) { // 100.64/10 (CGNAT, Tailscale)
		return errors.New("address not allowed")
	}
	return nil
}

var webClient = &http.Client{
	Timeout: 20 * time.Second,
	Transport: &http.Transport{
		Proxy:                 nil,
		DialContext:           (&net.Dialer{Timeout: 8 * time.Second, Control: publicOnly}).DialContext,
		TLSHandshakeTimeout:   8 * time.Second,
		ResponseHeaderTimeout: 12 * time.Second,
		MaxIdleConns:          4,
		IdleConnTimeout:       30 * time.Second,
	},
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("too many redirects")
		}
		if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
			return errors.New("scheme not allowed")
		}
		return nil
	},
}

func textOf(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return strings.Join(strings.Fields(b.String()), " ")
}

func hasClass(n *html.Node, cls string) bool {
	for _, a := range n.Attr {
		if a.Key == "class" {
			for _, c := range strings.Fields(a.Val) {
				if c == cls {
					return true
				}
			}
		}
	}
	return false
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

// cleanDDGURL unwraps DuckDuckGo redirect links (//duckduckgo.com/l/?uddg=...).
func cleanDDGURL(h string) string {
	if strings.HasPrefix(h, "//") {
		h = "https:" + h
	}
	if u, err := url.Parse(h); err == nil && strings.HasSuffix(u.Host, "duckduckgo.com") && u.Path == "/l/" {
		if t := u.Query().Get("uddg"); t != "" {
			return t
		}
	}
	return h
}

func webSearch(ctx context.Context, query, region string, max int) ([]webResult, error) {
	form := url.Values{"q": {query}}
	if region != "" {
		form.Set("kl", region)
	}
	req, _ := http.NewRequestWithContext(ctx, "POST", "https://html.duckduckgo.com/html/", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", webUA)
	resp, err := webClient.Do(req)
	if err != nil {
		return nil, errors.New("search_unreachable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("search: %s", resp.Status)
	}
	doc, err := html.Parse(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	var out []webResult
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if len(out) >= max {
			return
		}
		if n.Type == html.ElementNode && n.Data == "div" && hasClass(n, "result") && !hasClass(n, "result--ad") {
			var r webResult
			var find func(*html.Node)
			find = func(c *html.Node) {
				if c.Type == html.ElementNode {
					if c.Data == "a" && hasClass(c, "result__a") {
						r.Title, r.URL = textOf(c), cleanDDGURL(attr(c, "href"))
					} else if hasClass(c, "result__snippet") {
						r.Snippet = textOf(c)
					}
				}
				for k := c.FirstChild; k != nil; k = k.NextSibling {
					find(k)
				}
			}
			find(n)
			if r.URL != "" && r.Title != "" && !strings.Contains(r.URL, "duckduckgo.com/y.js") {
				out = append(out, r)
			}
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return out, nil
}

// readWebpage downloads a public page and returns its title and readable text.
func readWebpage(ctx context.Context, raw string) (string, string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", "", errors.New("invalid_url")
	}
	req, _ := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	req.Header.Set("User-Agent", webUA)
	req.Header.Set("Accept", "text/html,text/plain;q=0.9,*/*;q=0.5")
	resp, err := webClient.Do(req)
	if err != nil {
		return "", "", errors.New("page_unreachable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", "", fmt.Errorf("page: %s", resp.Status)
	}
	ct := resp.Header.Get("Content-Type")
	body := io.LimitReader(resp.Body, 3<<20)
	if strings.HasPrefix(ct, "text/plain") || strings.Contains(ct, "markdown") || strings.Contains(ct, "json") {
		b, _ := io.ReadAll(body)
		return "", truncateN(string(b), 20000), nil
	}
	if !strings.Contains(ct, "html") {
		return "", "", errors.New("unsupported content type: " + ct)
	}
	doc, err := html.Parse(body)
	if err != nil {
		return "", "", err
	}
	var title string
	var b strings.Builder
	skip := map[string]bool{"script": true, "style": true, "noscript": true, "svg": true, "nav": true, "footer": true, "header": true, "form": true, "iframe": true, "template": true, "aside": true}
	block := map[string]bool{"p": true, "div": true, "li": true, "h1": true, "h2": true, "h3": true, "h4": true, "tr": true, "br": true, "section": true, "article": true, "td": true}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			if n.Data == "title" && title == "" {
				title = textOf(n)
				return
			}
			if skip[n.Data] {
				return
			}
		}
		if n.Type == html.TextNode {
			if t := strings.Join(strings.Fields(n.Data), " "); t != "" {
				b.WriteString(t)
				b.WriteByte(' ')
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
		if n.Type == html.ElementNode && block[n.Data] {
			b.WriteByte('\n')
		}
	}
	walk(doc)
	lines := []string{}
	for _, l := range strings.Split(b.String(), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			lines = append(lines, l)
		}
	}
	return title, truncateN(strings.Join(lines, "\n"), 20000), nil
}

func truncateN(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "\n…[truncated]"
}
