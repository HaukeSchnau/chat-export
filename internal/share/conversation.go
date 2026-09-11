// Package share loads a public ChatGPT share page into a typed conversation.
package share

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/haukeschnau/chatgpt-exporter/internal/turbostream"
)

// Conversation is the subset of the share payload the exporter relies on.
// Unknown fields are ignored so upstream additions do not break parsing.
type Conversation struct {
	Title              string          `json:"title"`
	CreateTime         float64         `json:"create_time"`
	UpdateTime         float64         `json:"update_time"`
	ConversationID     string          `json:"conversation_id"`
	DefaultModelSlug   string          `json:"default_model_slug"`
	CurrentNode        string          `json:"current_node"`
	Mapping            map[string]Node `json:"mapping"`
	LinearConversation []Node          `json:"linear_conversation"`
}

type Node struct {
	ID       string   `json:"id"`
	Parent   string   `json:"parent"`
	Children []string `json:"children"`
	Message  *Message `json:"message"`
}

type Message struct {
	ID         string   `json:"id"`
	Author     Author   `json:"author"`
	CreateTime float64  `json:"create_time"`
	Content    Content  `json:"content"`
	Recipient  string   `json:"recipient"`
	Metadata   Metadata `json:"metadata"`
}

type Author struct {
	Role string `json:"role"`
	Name string `json:"name"`
}

// Content is a union over ChatGPT content types; which fields are populated
// depends on ContentType.
type Content struct {
	ContentType string `json:"content_type"`
	// text, multimodal_text: strings or attachment objects
	Parts []json.RawMessage `json:"parts"`
	// code, execution_output, system_error, tether_quote and others
	Text     string `json:"text"`
	Language string `json:"language"`
	// reasoning_recap
	Content string `json:"content"`
	// thoughts
	Thoughts []Thought `json:"thoughts"`
	// tether_browsing_display
	Result string `json:"result"`
}

type Thought struct {
	Summary string `json:"summary"`
	Content string `json:"content"`
}

type Metadata struct {
	IsVisuallyHidden   bool               `json:"is_visually_hidden_from_conversation"`
	IsThinkingPreamble bool               `json:"is_thinking_preamble_message"`
	ModelSlug          string             `json:"model_slug"`
	FinishedDurationS  float64            `json:"finished_duration_sec"`
	ContentReferences  []ContentReference `json:"content_references"`
	Attachments        []Attachment       `json:"attachments"`
}

// Attachment describes a file the user uploaded with a message.
type Attachment struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	MimeType string `json:"mimeType"`
}

// ContentReference describes a citation marker embedded in message text.
// StartIdx and EndIdx are UTF-16 code unit offsets, as produced by JavaScript.
type ContentReference struct {
	Type        string      `json:"type"`
	MatchedText string      `json:"matched_text"`
	StartIdx    int         `json:"start_idx"`
	EndIdx      int         `json:"end_idx"`
	Alt         string      `json:"alt"`
	Items       []RefSource `json:"items"`   // grouped_webpages
	Sources     []RefSource `json:"sources"` // sources_footnote
}

type RefSource struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Attribution string `json:"attribution"`
}

// Part is one element of a text or multimodal message: either text or an image.
type Part struct {
	Text  string
	Image *ImagePart
}

// ImagePart is an image_asset_pointer part. FileID is the bare file
// identifier without the sediment:// or file-service:// scheme.
type ImagePart struct {
	FileID string
	Width  int
	Height int
	Alt    string
}

// Parts decodes the message parts. Non-image attachments such as audio are
// reported as text placeholders.
func (m *Message) Parts() []Part {
	out := make([]Part, 0, len(m.Content.Parts))
	for _, raw := range m.Content.Parts {
		var s string
		if err := json.Unmarshal(raw, &s); err == nil {
			out = append(out, Part{Text: s})
			continue
		}
		var part struct {
			ContentType  string `json:"content_type"`
			AssetPointer string `json:"asset_pointer"`
			Width        int    `json:"width"`
			Height       int    `json:"height"`
			Metadata     struct {
				DallE *struct {
					Prompt string `json:"prompt"`
				} `json:"dalle"`
			} `json:"metadata"`
		}
		if err := json.Unmarshal(raw, &part); err != nil || part.ContentType != "image_asset_pointer" {
			label := part.ContentType
			if label == "" {
				label = "attachment"
			}
			out = append(out, Part{Text: fmt.Sprintf("*[%s not included in share]*", label)})
			continue
		}
		img := &ImagePart{FileID: FileID(part.AssetPointer), Width: part.Width, Height: part.Height, Alt: "image"}
		if part.Metadata.DallE != nil && part.Metadata.DallE.Prompt != "" {
			img.Alt = part.Metadata.DallE.Prompt
		}
		for _, a := range m.Metadata.Attachments {
			if a.ID == img.FileID && a.Name != "" {
				img.Alt = a.Name
			}
		}
		out = append(out, Part{Image: img})
	}
	return out
}

// Load parses the share page HTML and validates that the conversation is
// complete. Warnings about nodes outside the shared path are returned so the
// caller can surface them without failing.
func Load(html string) (*Conversation, []string, error) {
	stream, err := ExtractStream(html)
	if err != nil {
		return nil, nil, err
	}
	root, err := turbostream.Decode(stream)
	if err != nil {
		return nil, nil, err
	}
	data, err := findConversationData(root)
	if err != nil {
		return nil, nil, err
	}
	// Round-trip through JSON to get typed access with unknown fields dropped.
	encoded, err := json.Marshal(data)
	if err != nil {
		return nil, nil, err
	}
	var conv Conversation
	if err := json.Unmarshal(encoded, &conv); err != nil {
		return nil, nil, fmt.Errorf("conversation has unexpected shape: %w", err)
	}
	warnings, err := conv.validate()
	if err != nil {
		return nil, nil, err
	}
	return &conv, warnings, nil
}

// findConversationData navigates loaderData → share route → serverResponse.data.
// The route key is matched by prefix because React Router route IDs encode the
// file path, which may change.
func findConversationData(root any) (any, error) {
	rootObj, _ := root.(map[string]any)
	loaderData, _ := rootObj["loaderData"].(map[string]any)
	for key, value := range loaderData {
		if !strings.HasPrefix(key, "routes/share") {
			continue
		}
		route, _ := value.(map[string]any)
		response, _ := route["serverResponse"].(map[string]any)
		if data, ok := response["data"].(map[string]any); ok {
			return data, nil
		}
	}
	return nil, fmt.Errorf("page contains no shared conversation (is the link public and still valid?)")
}

// validate checks that linear_conversation is exactly the path from the root
// to current_node and reports any mapping nodes that lie off that path.
func (c *Conversation) validate() ([]string, error) {
	if len(c.Mapping) == 0 || c.CurrentNode == "" {
		return nil, fmt.Errorf("conversation has no messages")
	}
	var path []string
	seen := map[string]bool{}
	for id := c.CurrentNode; id != ""; {
		node, ok := c.Mapping[id]
		if !ok {
			return nil, fmt.Errorf("node %s referenced but missing from mapping", id)
		}
		if seen[id] {
			return nil, fmt.Errorf("cycle in conversation tree at %s", id)
		}
		seen[id] = true
		path = append(path, id)
		id = node.Parent
	}
	if len(path) != len(c.LinearConversation) {
		return nil, fmt.Errorf("linear conversation has %d nodes but the path to current_node has %d", len(c.LinearConversation), len(path))
	}
	for i, node := range c.LinearConversation {
		if node.ID != path[len(path)-1-i] {
			return nil, fmt.Errorf("linear conversation diverges from the tree at position %d", i)
		}
	}
	var warnings []string
	if extra := len(c.Mapping) - len(path); extra > 0 {
		warnings = append(warnings, fmt.Sprintf("%d nodes in the conversation tree are on branches not included in the shared path", extra))
	}
	return warnings, nil
}
