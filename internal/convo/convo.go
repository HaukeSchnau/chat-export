// Package convo is the provider-neutral conversation model. Each supported
// chat service parses its share format into this model, and the Markdown
// renderer consumes only this model.
package convo

import (
	"context"
	"time"
)

type Role string

const (
	User      Role = "user"
	Assistant Role = "assistant"
)

type Conversation struct {
	Provider      string // "chatgpt" or "claude"
	AssistantName string // heading label for assistant turns
	Title         string
	SourceURL     string
	Model         string    // empty when the provider does not expose it
	Created       time.Time // first message
	Updated       time.Time // last message
	Shared        time.Time // when the share link was created
	Turns         []Turn
}

// Turn is a run of blocks from one side. Consecutive blocks with the same
// role merge into one turn.
type Turn struct {
	Role   Role
	Blocks []Block
}

type Kind int

const (
	KindText    Kind = iota // Markdown text
	KindCode                // fenced source
	KindThought             // reasoning shown only on request
	KindImage               // downloadable image
	KindSources             // list of cited sources
)

type Block struct {
	Kind     Kind
	Text     string    // Text: Markdown; Code: source
	Language string    // Code
	Thoughts []Thought // Thought
	Image    *ImageRef // Image
	Sources  []Source  // Sources
}

type Thought struct {
	Summary string
	Content string
}

// ImageRef is an image that can be fetched on demand. ID must be unique
// within the conversation and safe to use as a file name.
type ImageRef struct {
	ID       string
	Alt      string
	Download func(ctx context.Context) (data []byte, contentType string, err error)
}

type Source struct {
	Title string
	URL   string
}

// Add appends blocks to the current turn when it has the same role, and
// starts a new turn otherwise.
func (c *Conversation) Add(role Role, blocks ...Block) {
	if len(blocks) == 0 {
		return
	}
	if n := len(c.Turns); n > 0 && c.Turns[n-1].Role == role {
		c.Turns[n-1].Blocks = append(c.Turns[n-1].Blocks, blocks...)
		return
	}
	c.Turns = append(c.Turns, Turn{Role: role, Blocks: blocks})
}

// Touch widens the created/updated range to include t.
func (c *Conversation) Touch(t time.Time) {
	if t.IsZero() {
		return
	}
	if c.Created.IsZero() || t.Before(c.Created) {
		c.Created = t
	}
	if t.After(c.Updated) {
		c.Updated = t
	}
}
