// Package markdown renders a shared conversation as a Markdown document that
// mirrors what the ChatGPT web UI shows: user prompts and final assistant
// answers, with citation markers turned into links.
package markdown

import (
	"fmt"
	"strings"
	"time"

	"github.com/haukeschnau/chatgpt-exporter/internal/share"
)

type Options struct {
	SourceURL string
	// IncludeThoughts adds reasoning summaries and thinking preambles as
	// blockquotes before each answer.
	IncludeThoughts bool
	// ImageSrc returns the link target for an image, typically a path relative
	// to the output file. Return "" to emit a placeholder instead. When nil,
	// every image becomes a placeholder.
	ImageSrc func(share.ImagePart) string
}

// Render produces the document. Warnings describe content that could not be
// rendered faithfully, such as unknown content types.
func Render(conv *share.Conversation, opts Options) (string, []string) {
	r := &renderer{opts: opts}
	r.frontMatter(conv)
	fmt.Fprintf(&r.out, "# %s\n", conv.Title)

	for _, node := range conv.LinearConversation {
		msg := node.Message
		if msg == nil || msg.Metadata.IsVisuallyHidden || msg.Recipient != "all" {
			continue
		}
		switch msg.Author.Role {
		case "user":
			r.section("User")
			body, _ := r.parts(msg)
			r.paragraph(body)
		case "assistant":
			r.assistant(msg)
		case "tool":
			// Generated images arrive as tool output; the UI shows them as
			// part of the answer. Tool text is never shown.
			r.toolImages(msg)
		}
	}
	return r.out.String(), r.warnings
}

type renderer struct {
	opts     Options
	out      strings.Builder
	warnings []string
	lastRole string
}

// frontMatter emits YAML metadata. The payload's own create_time is when the
// share link was made, so created and updated come from the first and last
// message timestamps instead.
func (r *renderer) frontMatter(conv *share.Conversation) {
	var first, last float64
	for _, node := range conv.LinearConversation {
		if node.Message == nil || node.Message.CreateTime == 0 {
			continue
		}
		if first == 0 || node.Message.CreateTime < first {
			first = node.Message.CreateTime
		}
		if node.Message.CreateTime > last {
			last = node.Message.CreateTime
		}
	}
	fmt.Fprintf(&r.out, "---\ntitle: %s\nsource: %s\nmodel: %s\ncreated: %s\nupdated: %s\nshared: %s\n---\n\n",
		yamlString(conv.Title), r.opts.SourceURL, conv.DefaultModelSlug,
		formatTime(first), formatTime(last), formatTime(conv.CreateTime))
}

// section starts a new turn heading unless the previous turn had the same role,
// so consecutive assistant messages merge into one answer.
func (r *renderer) section(role string) {
	if r.lastRole == role {
		return
	}
	r.lastRole = role
	fmt.Fprintf(&r.out, "\n## %s\n", role)
}

func (r *renderer) paragraph(body string) {
	body = strings.TrimSpace(body)
	if body == "" {
		return
	}
	r.out.WriteString("\n" + body + "\n")
}

func (r *renderer) assistant(msg *share.Message) {
	c := msg.Content
	switch c.ContentType {
	case "thoughts":
		if r.opts.IncludeThoughts {
			r.section("ChatGPT")
			r.paragraph(renderThoughts(c.Thoughts))
		}
	case "reasoning_recap":
		if r.opts.IncludeThoughts {
			r.section("ChatGPT")
			r.paragraph("*" + c.Content + "*")
		}
	case "text", "multimodal_text":
		if msg.Metadata.IsThinkingPreamble && !r.opts.IncludeThoughts {
			return
		}
		body, sources := r.parts(msg)
		body = demoteHeadings(body)
		if msg.Metadata.IsThinkingPreamble {
			body = blockquote(body)
		}
		if strings.TrimSpace(body) == "" {
			return
		}
		r.section("ChatGPT")
		r.paragraph(body)
		if len(sources) > 0 {
			r.paragraph("**Sources**\n\n" + sourceList(sources))
		}
	case "code":
		r.section("ChatGPT")
		r.paragraph(fence(c.Language, c.Text))
	case "execution_output":
		r.section("ChatGPT")
		r.paragraph(fence("", c.Text))
	default:
		r.warnings = append(r.warnings, fmt.Sprintf("skipped message %s with unsupported content type %q", msg.ID, c.ContentType))
	}
}

// parts renders a message's image and text parts. Citation markers are
// spliced into the text before images are prepended, so reference offsets,
// which refer to the text alone, stay valid.
func (r *renderer) parts(msg *share.Message) (string, []share.RefSource) {
	var images, texts []string
	for _, p := range msg.Parts() {
		if p.Image == nil {
			texts = append(texts, p.Text)
			continue
		}
		images = append(images, r.image(*p.Image))
	}
	body, sources := spliceReferences(strings.Join(texts, "\n\n"), msg.Metadata.ContentReferences)
	if body != "" {
		images = append(images, body)
	}
	return strings.Join(images, "\n\n"), sources
}

func (r *renderer) toolImages(msg *share.Message) {
	if msg.Content.ContentType != "multimodal_text" {
		return
	}
	var images []string
	for _, p := range msg.Parts() {
		if p.Image != nil {
			images = append(images, r.image(*p.Image))
		}
	}
	if len(images) > 0 {
		r.section("ChatGPT")
		r.paragraph(strings.Join(images, "\n\n"))
	}
}

// image renders one image part as a Markdown image, or a placeholder when
// it cannot be resolved.
func (r *renderer) image(img share.ImagePart) string {
	src := ""
	if r.opts.ImageSrc != nil {
		src = r.opts.ImageSrc(img)
	}
	if src == "" {
		return fmt.Sprintf("*[image: %s]*", img.Alt)
	}
	return fmt.Sprintf("![%s](%s)", img.Alt, src)
}

func renderThoughts(thoughts []share.Thought) string {
	var parts []string
	for _, t := range thoughts {
		if strings.TrimSpace(t.Content) == "" {
			continue
		}
		parts = append(parts, "**"+t.Summary+"**\n\n"+t.Content)
	}
	return blockquote(strings.Join(parts, "\n\n"))
}

func blockquote(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight("> "+line, " ")
	}
	return strings.Join(lines, "\n")
}

func fence(lang, code string) string {
	return "```" + lang + "\n" + strings.TrimRight(code, "\n") + "\n```"
}

func sourceList(sources []share.RefSource) string {
	var b strings.Builder
	seen := map[string]bool{}
	for _, s := range sources {
		url := cleanURL(s.URL)
		if seen[url] {
			continue
		}
		seen[url] = true
		title := s.Title
		if title == "" {
			title = s.Attribution
		}
		fmt.Fprintf(&b, "- [%s](%s)\n", title, url)
	}
	return strings.TrimRight(b.String(), "\n")
}

// demoteHeadings pushes every ATX heading outside fenced code blocks one level
// down so answer headings nest under the "## ChatGPT" turn heading.
func demoteHeadings(body string) string {
	lines := strings.Split(body, "\n")
	inFence := false
	for i, line := range lines {
		trimmed := strings.TrimLeft(line, " ")
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence || !strings.HasPrefix(line, "#") {
			continue
		}
		level := len(line) - len(strings.TrimLeft(line, "#"))
		if level < 6 && strings.HasPrefix(line[level:], " ") {
			lines[i] = "#" + line
		}
	}
	return strings.Join(lines, "\n")
}

func formatTime(unixSeconds float64) string {
	if unixSeconds == 0 {
		return ""
	}
	sec, frac := int64(unixSeconds), unixSeconds-float64(int64(unixSeconds))
	return time.Unix(sec, int64(frac*1e9)).UTC().Format(time.RFC3339)
}

func yamlString(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}
