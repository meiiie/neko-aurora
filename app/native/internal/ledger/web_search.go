package ledger

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// web.search runs a public DuckDuckGo query so chat tasks can ground
// answers in live pages. It is read-only: no account, no stored state,
// and the fetch happens before the store lock is taken.
var webSearchClient = &http.Client{Timeout: 8 * time.Second}

var (
	webResultAnchor = regexp.MustCompile(`(?s)<a[^>]*class="[^"]*result__a[^"]*"[^>]*href="([^"]+)"[^>]*>(.*?)</a>`)
	webSnippetLine  = regexp.MustCompile(`(?s)<a[^>]*class="[^"]*result__snippet[^"]*"[^>]*>(.*?)</a>`)
	webTagStrip     = regexp.MustCompile(`<[^>]+>`)
)

func decodeWebEntities(value string) string {
	replacer := strings.NewReplacer("&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", `"`, "&#x27;", "'", "&#39;", "'", "&#47;", "/", "&nbsp;", " ")
	return replacer.Replace(value)
}

func webSearchError(code string) json.RawMessage {
	return textResult(map[string]any{"error": code})
}

func webSearchResults(query string) json.RawMessage {
	request, err := http.NewRequest(http.MethodGet, "https://html.duckduckgo.com/html/?q="+url.QueryEscape(query), nil)
	if err != nil {
		return webSearchError("search_request_failed")
	}
	request.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0 Safari/537.36")
	request.Header.Set("Accept-Language", "vi,en;q=0.8")
	response, err := webSearchClient.Do(request)
	if err != nil {
		return webSearchError("search_network_failed")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return webSearchError("search_upstream_unavailable")
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 512<<10))
	if err != nil {
		return webSearchError("search_read_failed")
	}
	anchors := webResultAnchor.FindAllStringSubmatch(string(body), -1)
	snippets := webSnippetLine.FindAllStringSubmatch(string(body), -1)
	results := []map[string]string{}
	for index, anchor := range anchors {
		if len(results) >= 5 {
			break
		}
		link := decodeWebEntities(anchor[1])
		if marker := strings.Index(link, "duckduckgo.com/l/"); marker >= 0 {
			candidate := link
			if strings.HasPrefix(candidate, "//") {
				candidate = "https:" + candidate
			} else if marker > 0 {
				candidate = "https://" + link[:strings.Index(link, "duckduckgo.com/l/")] + "duckduckgo.com/l/" + link[marker+len("duckduckgo.com/l/"):]
			}
			if parsed, parseErr := url.Parse(candidate); parseErr == nil {
				if target := parsed.Query().Get("uddg"); target != "" {
					link = target
				}
			}
		}
		title := strings.TrimSpace(webTagStrip.ReplaceAllString(anchor[2], ""))
		title = decodeWebEntities(title)
		snippet := ""
		if index < len(snippets) {
			snippet = decodeWebEntities(strings.TrimSpace(webTagStrip.ReplaceAllString(snippets[index][1], "")))
		}
		if title == "" || !strings.HasPrefix(link, "http") || len(title) > 300 || len(snippet) > 500 {
			continue
		}
		results = append(results, map[string]string{"title": title, "url": link, "snippet": snippet})
	}
	if len(results) == 0 {
		return webSearchError("search_no_results")
	}
	return textResult(map[string]any{"query": query, "source": "duckduckgo.com", "results": results})
}
