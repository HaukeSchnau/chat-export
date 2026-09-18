package claude

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/haukeschnau/chatgpt-exporter/internal/convo"
)

// Snapshot is the subset of the chat_snapshots payload the exporter uses.
type Snapshot struct {
	UUID         string    `json:"uuid"`
	SnapshotName string    `json:"snapshot_name"`
	CreatedAt    time.Time `json:"created_at"`
	ChatMessages []Message `json:"chat_messages"`
}

type Message struct {
	UUID              string    `json:"uuid"`
	Sender            string    `json:"sender"` // "human" or "assistant"
	Index             int       `json:"index"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
	ParentMessageUUID string    `json:"parent_message_uuid"`
	Text              string    `json:"text"` // legacy flat text, used when Content is empty
	Content           []Content `json:"content"`
	FileCount         int       `json:"file_count"`
	ImageCount        int       `json:"image_count"`
}

// Content is a union over block types; which fields apply depends on Type.
type Content struct {
	Type string `json:"type"`
	// text
	Text string `json:"text"`
	// thinking
	Thinking  string `json:"thinking"`
	Summaries []struct {
		Summary string `json:"summary"`
	} `json:"summaries"`
	// tool_use and tool_result; shares strip inputs and outputs but keep
	// display cards for artifacts
	Name           string `json:"name"`
	IsError        bool   `json:"is_error"`
	DisplayContent *struct {
		Type         string `json:"type"`
		Title        string `json:"title"`
		PublishedURL string `json:"published_url"`
	} `json:"display_content"`
}

const rootParent = "00000000-0000-4000-8000-000000000000"

// Parse decodes a snapshot and checks that its messages form one linear
// chain from the root, so nothing is silently missing or reordered.
func Parse(data []byte) (*Snapshot, error) {
	var s Snapshot
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("snapshot has unexpected shape: %w", err)
	}
	if len(s.ChatMessages) == 0 {
		return nil, fmt.Errorf("snapshot has no messages")
	}
	sort.SliceStable(s.ChatMessages, func(i, j int) bool { return s.ChatMessages[i].Index < s.ChatMessages[j].Index })
	previous := rootParent
	for i, m := range s.ChatMessages {
		if m.ParentMessageUUID != previous {
			return nil, fmt.Errorf("message %d does not continue the previous message; the snapshot is not a linear conversation", i)
		}
		previous = m.UUID
	}
	return &s, nil
}

// Export converts the snapshot into the neutral model. Tool calls are not
// shown, except artifact cards, which become links. Attachments are not part
// of a share, so they become placeholders and warnings.
func (s *Snapshot) Export() (*convo.Conversation, []string) {
	out := &convo.Conversation{
		Provider:      "claude",
		AssistantName: "Claude",
		Title:         s.SnapshotName,
		SourceURL:     ShareURL(s.UUID),
		Shared:        s.CreatedAt.UTC(),
	}
	var warnings []string
	for _, m := range s.ChatMessages {
		out.Touch(m.CreatedAt.UTC())
		out.Touch(m.UpdatedAt.UTC())
		role := convo.Assistant
		if m.Sender == "human" {
			role = convo.User
		}
		var blocks []convo.Block
		if n := m.FileCount + m.ImageCount; n > 0 {
			blocks = append(blocks, convo.Block{Kind: convo.KindText, Text: fmt.Sprintf("*[%s not included in share]*", plural(n, "attachment"))})
			warnings = append(warnings, fmt.Sprintf("message %d has %s that the share does not include", m.Index, plural(n, "attachment")))
		}
		if len(m.Content) == 0 && strings.TrimSpace(m.Text) != "" {
			blocks = append(blocks, convo.Block{Kind: convo.KindText, Text: m.Text})
		}
		for _, c := range m.Content {
			switch c.Type {
			case "text":
				if strings.TrimSpace(c.Text) != "" {
					blocks = append(blocks, convo.Block{Kind: convo.KindText, Text: c.Text})
				}
			case "thinking":
				if strings.TrimSpace(c.Thinking) == "" {
					continue
				}
				var summary []string
				for _, sm := range c.Summaries {
					summary = append(summary, sm.Summary)
				}
				blocks = append(blocks, convo.Block{Kind: convo.KindThought, Thoughts: []convo.Thought{{Summary: strings.Join(summary, " · "), Content: c.Thinking}}})
			case "tool_use":
			case "tool_result":
				if d := c.DisplayContent; d != nil && d.Type == "file" && d.PublishedURL != "" && !c.IsError {
					title := d.Title
					if title == "" {
						title = d.PublishedURL
					}
					blocks = append(blocks, convo.Block{Kind: convo.KindText, Text: fmt.Sprintf("*Artifact: [%s](%s)*", title, d.PublishedURL)})
				}
			default:
				warnings = append(warnings, fmt.Sprintf("skipped unsupported content type %q in message %d", c.Type, m.Index))
			}
		}
		out.Add(role, blocks...)
	}
	return out, warnings
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}
