// Package claude loads a public claude.ai share (a "chat snapshot") and
// converts it to the neutral model.
package claude

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"regexp"
	"strings"
)

var shareIDPattern = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)

// ParseShareID accepts a full share URL or a bare snapshot ID.
func ParseShareID(input string) (string, error) {
	id := shareIDPattern.FindString(strings.ToLower(input))
	if id == "" {
		return "", fmt.Errorf("no share ID found in %q", input)
	}
	return id, nil
}

func ShareURL(id string) string {
	return "https://claude.ai/share/" + id
}

// FetchSnapshot downloads the snapshot JSON behind a share link. The share
// page itself is an empty app shell; the data comes from this endpoint.
//
// Cloudflare serves a browser challenge to requests on the API path that
// carry no Sec-Fetch headers, regardless of client IP, so the request mimics
// a same-origin fetch. If that heuristic changes, the response is an HTML
// challenge page and this returns a descriptive error.
func FetchSnapshot(ctx context.Context, id string) ([]byte, error) {
	target := "https://claude.ai/api/chat_snapshots/" + id + "?rendering_mode=messages&render_all_tools=true"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "chatgpt-exporter (+https://github.com/haukeschnau/chatgpt-exporter)")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Site", "same-origin")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	mediaType, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("share %s not found (is the link public and still valid?)", id)
	}
	if resp.StatusCode != http.StatusOK || mediaType != "application/json" {
		if strings.Contains(string(body), "challenges.cloudflare.com") {
			return nil, fmt.Errorf("GET %s: blocked by a Cloudflare browser challenge (%s)", target, resp.Status)
		}
		return nil, fmt.Errorf("GET %s: %s (%s)", target, resp.Status, mediaType)
	}
	return body, nil
}
