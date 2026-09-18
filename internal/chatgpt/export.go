package chatgpt

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/haukeschnau/chat-export/internal/convo"
)

// Export converts the share payload into the neutral model, mirroring what
// the ChatGPT web UI shows: visible user and assistant messages, generated
// images from tool output, and reasoning as Thought blocks. Warnings name
// content that was skipped.
func (c *Conversation) Export() (*convo.Conversation, []string) {
	out := &convo.Conversation{
		Provider:      "chatgpt",
		AssistantName: "ChatGPT",
		Title:         c.Title,
		SourceURL:     ShareURL(c.ConversationID),
		Model:         c.DefaultModelSlug,
		Shared:        unixTime(c.CreateTime),
	}
	var warnings []string
	for _, node := range c.LinearConversation {
		msg := node.Message
		if msg == nil || msg.Metadata.IsVisuallyHidden || msg.Recipient != "all" {
			continue
		}
		switch msg.Author.Role {
		case "user":
			out.Touch(unixTime(msg.CreateTime))
			out.Add(convo.User, c.parts(msg)...)
		case "assistant":
			out.Touch(unixTime(msg.CreateTime))
			blocks, warning := c.assistant(msg)
			if warning != "" {
				warnings = append(warnings, warning)
			}
			out.Add(convo.Assistant, blocks...)
		case "tool":
			// Generated images arrive as tool output; the UI shows them as
			// part of the answer. Tool text is never shown.
			if msg.Content.ContentType == "multimodal_text" {
				for _, b := range c.parts(msg) {
					if b.Kind == convo.KindImage {
						out.Add(convo.Assistant, b)
					}
				}
			}
		}
	}
	return out, warnings
}

func (c *Conversation) assistant(msg *Message) ([]convo.Block, string) {
	content := msg.Content
	switch content.ContentType {
	case "thoughts":
		var thoughts []convo.Thought
		for _, t := range content.Thoughts {
			if strings.TrimSpace(t.Content) != "" {
				thoughts = append(thoughts, convo.Thought{Summary: t.Summary, Content: t.Content})
			}
		}
		if len(thoughts) == 0 {
			return nil, ""
		}
		return []convo.Block{{Kind: convo.KindThought, Thoughts: thoughts}}, ""
	case "reasoning_recap":
		return []convo.Block{{Kind: convo.KindThought, Thoughts: []convo.Thought{{Content: content.Content}}}}, ""
	case "text", "multimodal_text":
		if msg.Metadata.IsThinkingPreamble {
			text := strings.Join(textParts(msg), "\n\n")
			return []convo.Block{{Kind: convo.KindThought, Thoughts: []convo.Thought{{Content: text}}}}, ""
		}
		return c.parts(msg), ""
	case "code":
		return []convo.Block{{Kind: convo.KindCode, Language: content.Language, Text: content.Text}}, ""
	case "execution_output":
		return []convo.Block{{Kind: convo.KindCode, Text: content.Text}}, ""
	default:
		return nil, fmt.Sprintf("skipped message %s with unsupported content type %q", msg.ID, content.ContentType)
	}
}

// parts turns a message's parts into blocks: images first, then the text
// with citations spliced in, then the sources it cited.
func (c *Conversation) parts(msg *Message) []convo.Block {
	var blocks []convo.Block
	for _, p := range msg.Parts() {
		if p.Image == nil {
			continue
		}
		img := *p.Image
		shareID := c.ConversationID
		blocks = append(blocks, convo.Block{Kind: convo.KindImage, Image: &convo.ImageRef{
			ID:  img.FileID,
			Alt: img.Alt,
			Download: func(ctx context.Context) ([]byte, string, error) {
				return DownloadFile(ctx, shareID, img.FileID)
			},
		}})
	}
	body, sources := spliceReferences(strings.Join(textParts(msg), "\n\n"), msg.Metadata.ContentReferences)
	if strings.TrimSpace(body) != "" {
		blocks = append(blocks, convo.Block{Kind: convo.KindText, Text: body})
	}
	if len(sources) > 0 {
		var list []convo.Source
		for _, s := range sources {
			title := s.Title
			if title == "" {
				title = s.Attribution
			}
			list = append(list, convo.Source{Title: title, URL: cleanURL(s.URL)})
		}
		blocks = append(blocks, convo.Block{Kind: convo.KindSources, Sources: list})
	}
	return blocks
}

func textParts(msg *Message) []string {
	var texts []string
	for _, p := range msg.Parts() {
		if p.Image == nil {
			texts = append(texts, p.Text)
		}
	}
	return texts
}

func unixTime(seconds float64) time.Time {
	if seconds == 0 {
		return time.Time{}
	}
	sec := int64(seconds)
	return time.Unix(sec, int64((seconds-float64(sec))*1e9)).UTC()
}
