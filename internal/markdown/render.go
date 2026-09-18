// Package markdown renders the neutral conversation model as a Markdown
// document: front matter, a title, and one heading per turn.
package markdown

import (
	"fmt"
	"strings"
	"time"

	"github.com/haukeschnau/chatgpt-exporter/internal/convo"
)

type Options struct {
	// IncludeThoughts renders Thought blocks as blockquotes; otherwise they
	// are dropped.
	IncludeThoughts bool
	// ImageSrc returns the link target for an image, typically a path relative
	// to the output file. Return "" to emit a placeholder instead. When nil,
	// every image becomes a placeholder.
	ImageSrc func(convo.ImageRef) string
}

func Render(c *convo.Conversation, opts Options) string {
	var out strings.Builder
	frontMatter(&out, c)
	fmt.Fprintf(&out, "# %s\n", c.Title)
	for _, turn := range c.Turns {
		var parts []string
		for _, b := range turn.Blocks {
			if s := block(b, opts); s != "" {
				parts = append(parts, s)
			}
		}
		if len(parts) == 0 {
			continue
		}
		label := "User"
		if turn.Role == convo.Assistant {
			label = c.AssistantName
		}
		fmt.Fprintf(&out, "\n## %s\n\n%s\n", label, strings.Join(parts, "\n\n"))
	}
	return out.String()
}

func frontMatter(out *strings.Builder, c *convo.Conversation) {
	fields := [][2]string{
		{"title", yamlString(c.Title)},
		{"provider", c.Provider},
		{"source", c.SourceURL},
		{"model", c.Model},
		{"created", formatTime(c.Created)},
		{"updated", formatTime(c.Updated)},
		{"shared", formatTime(c.Shared)},
	}
	out.WriteString("---\n")
	for _, f := range fields {
		if f[1] != "" {
			fmt.Fprintf(out, "%s: %s\n", f[0], f[1])
		}
	}
	out.WriteString("---\n\n")
}

func block(b convo.Block, opts Options) string {
	switch b.Kind {
	case convo.KindText:
		return strings.TrimSpace(demoteHeadings(b.Text))
	case convo.KindCode:
		return "```" + b.Language + "\n" + strings.TrimRight(b.Text, "\n") + "\n```"
	case convo.KindThought:
		if !opts.IncludeThoughts {
			return ""
		}
		var parts []string
		for _, t := range b.Thoughts {
			switch {
			case t.Summary != "" && t.Content != "":
				parts = append(parts, "**"+t.Summary+"**\n\n"+t.Content)
			case t.Summary != "":
				parts = append(parts, "**"+t.Summary+"**")
			default:
				parts = append(parts, t.Content)
			}
		}
		return blockquote(strings.Join(parts, "\n\n"))
	case convo.KindImage:
		src := ""
		if opts.ImageSrc != nil {
			src = opts.ImageSrc(*b.Image)
		}
		if src == "" {
			return fmt.Sprintf("*[image: %s]*", b.Image.Alt)
		}
		return fmt.Sprintf("![%s](%s)", b.Image.Alt, src)
	case convo.KindSources:
		var list strings.Builder
		seen := map[string]bool{}
		list.WriteString("**Sources**\n")
		for _, s := range b.Sources {
			if seen[s.URL] {
				continue
			}
			seen[s.URL] = true
			fmt.Fprintf(&list, "\n- [%s](%s)", s.Title, s.URL)
		}
		return list.String()
	}
	return ""
}

func blockquote(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight("> "+line, " ")
	}
	return strings.Join(lines, "\n")
}

// demoteHeadings pushes every ATX heading outside fenced code blocks one level
// down so headings inside a message nest under the turn heading.
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

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func yamlString(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}
