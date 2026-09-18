package chatgpt

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// FileID strips the asset pointer scheme, leaving the file identifier the
// download endpoint expects.
func FileID(assetPointer string) string {
	id := strings.TrimPrefix(assetPointer, "sediment://")
	id = strings.TrimPrefix(id, "file-service://")
	// Share payloads may carry the share ID as a query suffix on the pointer.
	id, _, _ = strings.Cut(id, "?")
	return id
}

// deviceID identifies this run to the anonymous backend. Logged-out visitors
// get a random UUID from the web app, and the file endpoint rejects requests
// without one, so any fresh UUID per process serves the same purpose.
var deviceID = newUUID()

// DownloadFile fetches a file attached to a shared conversation the same way
// the share page does for logged-out visitors: ask the anonymous backend for a
// signed download URL scoped to the share, then fetch the bytes.
func DownloadFile(ctx context.Context, shareID, fileID string) (data []byte, contentType string, err error) {
	query := url.Values{"shared_conversation_id": {shareID}, "inline": {"false"}}
	endpoint := "https://chatgpt.com/backend-anon/files/download/" + url.PathEscape(fileID) + "?" + query.Encode()

	var meta struct {
		Status       string `json:"status"`
		DownloadURL  string `json:"download_url"`
		ErrorCode    string `json:"error_code"`
		ErrorMessage string `json:"error_message"`
		Detail       string `json:"detail"`
	}
	body, _, err := get(ctx, endpoint, "application/json")
	if err != nil {
		return nil, "", err
	}
	if err := json.Unmarshal(body, &meta); err != nil {
		return nil, "", fmt.Errorf("file %s: unexpected download metadata: %w", fileID, err)
	}
	if meta.DownloadURL == "" {
		reason := meta.ErrorMessage
		if reason == "" {
			reason = meta.ErrorCode
		}
		if reason == "" {
			reason = meta.Detail
		}
		return nil, "", fmt.Errorf("file %s: no download URL (%s)", fileID, reason)
	}
	return get(ctx, meta.DownloadURL, "*/*")
}

func get(ctx context.Context, target, accept string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", accept)
	req.Header.Set("oai-device-id", deviceID)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("GET %s: %s", target, resp.Status)
	}
	return body, resp.Header.Get("Content-Type"), nil
}

// newUUID returns a random RFC 4122 version 4 UUID.
func newUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
