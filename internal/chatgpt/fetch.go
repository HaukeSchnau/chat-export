package chatgpt

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// shareIDPattern matches the UUID that identifies a shared conversation.
var shareIDPattern = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)

// ParseShareID accepts a full share URL or a bare share ID and returns the ID.
func ParseShareID(input string) (string, error) {
	id := shareIDPattern.FindString(strings.ToLower(input))
	if id == "" {
		return "", fmt.Errorf("no share ID found in %q", input)
	}
	return id, nil
}

// ShareURL is the canonical public URL for a share ID.
func ShareURL(id string) string {
	return "https://chatgpt.com/share/" + id
}

const userAgent = "chatgpt-exporter (+https://github.com/haukeschnau/chatgpt-exporter)"

// FetchHTML downloads the server-rendered share page.
func FetchHTML(ctx context.Context, id string) (string, error) {
	body, _, err := get(ctx, ShareURL(id), "text/html")
	return string(body), err
}

const enqueuePrefix = `streamController.enqueue(`

// ExtractStream pulls the turbo-stream payload out of the share page HTML.
// React Router emits it as one or more streamController.enqueue("...") calls
// whose argument is a JSON string literal; the decoded strings concatenated
// form the stream.
func ExtractStream(html string) (string, error) {
	var stream strings.Builder
	rest := html
	for {
		start := strings.Index(rest, enqueuePrefix)
		if start < 0 {
			break
		}
		rest = rest[start+len(enqueuePrefix):]
		if !strings.HasPrefix(rest, `"`) {
			continue
		}
		end := jsonStringEnd(rest)
		if end < 0 {
			return "", fmt.Errorf("unterminated string literal in stream chunk")
		}
		var chunk string
		if err := json.Unmarshal([]byte(rest[:end+1]), &chunk); err != nil {
			return "", fmt.Errorf("decoding stream chunk: %w", err)
		}
		stream.WriteString(chunk)
		rest = rest[end+1:]
	}
	if stream.Len() == 0 {
		return "", fmt.Errorf("no embedded conversation data found in page")
	}
	return stream.String(), nil
}

// jsonStringEnd returns the index of the closing quote of the JSON string
// literal that starts at s[0], honoring backslash escapes.
func jsonStringEnd(s string) int {
	for i := 1; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case '"':
			return i
		}
	}
	return -1
}
