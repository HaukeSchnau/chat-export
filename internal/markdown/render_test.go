package markdown

import (
	"os"
	"strings"
	"testing"

	"github.com/haukeschnau/chatgpt-exporter/internal/chatgpt"
	"github.com/haukeschnau/chatgpt-exporter/internal/claude"
	"github.com/haukeschnau/chatgpt-exporter/internal/convo"
)

func TestDemoteHeadings(t *testing.T) {
	in := "## Title\n```md\n# not a heading\n```\n#hashtag\n###### six"
	want := "### Title\n```md\n# not a heading\n```\n#hashtag\n###### six"
	if got := demoteHeadings(in); got != want {
		t.Errorf("got %q", got)
	}
}

func expect(t *testing.T, doc string, wanted map[string]string, unwanted map[string]string) {
	t.Helper()
	for name, want := range wanted {
		if !strings.Contains(doc, want) {
			t.Errorf("%s: output lacks %q", name, want)
		}
	}
	for name, s := range unwanted {
		if strings.Contains(doc, s) {
			t.Errorf("%s: output contains %q", name, s)
		}
	}
}

func TestRenderChatGPT(t *testing.T) {
	html, err := os.ReadFile("../../testdata/share.html")
	if err != nil {
		t.Fatal(err)
	}
	raw, _, err := chatgpt.Load(string(html))
	if err != nil {
		t.Fatal(err)
	}
	conv, warnings := raw.Export()
	if len(warnings) != 0 {
		t.Errorf("warnings: %v", warnings)
	}
	var requested []string
	doc := Render(conv, Options{ImageSrc: func(img convo.ImageRef) string {
		requested = append(requested, img.ID)
		if strings.HasSuffix(img.ID, "2") {
			return "" // simulate a failed download
		}
		return "assets/" + img.ID + ".jpg"
	}})
	expect(t, doc, map[string]string{
		"front matter":        "---\ntitle: \"Fixture: Picnic Planning 🧺\"\nprovider: chatgpt\nsource: https://chatgpt.com/share/00000000-0000-4000-8000-000000000001\nmodel: fixture-model\n",
		"user image":          "## User\n\n![park.jpg](assets/file_00000000000000000000000001.jpg)\n\nPlan a picnic",
		"generated image":     "## ChatGPT\n\n*[image: Generated image]*\n\n```json",
		"demoted heading":     "\n## Picnic plan\n",
		"fenced heading kept": "```md\n# not a heading\n```",
		"single citation":     "shade. ([weather.example](https://weather.example/forecast)) Saturday",
		"grouped citation":    "dry. ([picnics.example](https://picnics.example/list), [picnics.example](https://picnics.example/blankets))",
		"sources list":        "**Sources**\n\n- [Weekend forecast](https://weather.example/forecast)\n- [Packing list](https://picnics.example/list)",
		"code message":        "```json\n{\"list\": [\"blanket\", \"fruit\"]}\n```",
		"final answer":        "Here is the drawing and the list. Enjoy! 😀",
	}, map[string]string{
		"stale marker":        "turn0view9",
		"hidden marker":       "memcite",
		"delimiter":           "",
		"tracking parameter":  "utm_source",
		"preamble":            "I will check the local weather",
		"tool output":         "redacted",
		"custom instructions": "custom instructions",
	})
	if got := strings.Count(doc, "\n## User\n"); got != 2 {
		t.Errorf("%d user turns, want 2", got)
	}
	if got := strings.Count(doc, "\n## ChatGPT\n"); got != 2 {
		t.Errorf("%d assistant turns, want 2", got)
	}
	if len(requested) != 2 {
		t.Errorf("image resolver called %d times, want 2", len(requested))
	}

	withThoughts := Render(conv, Options{IncludeThoughts: true})
	expect(t, withThoughts, map[string]string{
		"thought":  "> **Looking at the park**\n>\n> The photo shows",
		"preamble": "> I will check the local weather",
		"recap":    "> Worked for 4s",
	}, map[string]string{"empty thought": "Looked at the park"})
}

func TestRenderClaude(t *testing.T) {
	data, err := os.ReadFile("../../testdata/claude-snapshot.json")
	if err != nil {
		t.Fatal(err)
	}
	snap, err := claude.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	conv, _ := snap.Export()
	doc := Render(conv, Options{})
	expect(t, doc, map[string]string{
		"front matter":    "---\ntitle: \"Fixture: Planning a Picnic 🧺\"\nprovider: claude\nsource: https://claude.ai/share/00000000-0000-4000-8000-00000000c1a0\ncreated: 2024-01-01T09:00:00Z\nupdated: 2024-01-01T09:01:10Z\nshared: 2024-01-02T10:00:00Z\n---",
		"attachment":      "## User\n\n*[1 attachment not included in share]*\n\nPlan a picnic",
		"demoted heading": "## Claude\n\n## Picnic plan\n",
		"artifact link":   "*Artifact: [Picnic Checklist](https://claude.ai/code/artifact/00000000-0000-4000-8000-0000000000a1)*",
		"legacy text":     "## User\n\nThanks! Anything else?",
		"final":           "## Claude\n\nSunscreen. Enjoy! 😀",
	}, map[string]string{
		"model line": "model:",
		"thinking":   "shade trees",
		"tool name":  "Claude Docs",
	})
	withThoughts := Render(conv, Options{IncludeThoughts: true})
	expect(t, withThoughts, map[string]string{"thought": "> **Looking at the park**\n>\n> The photo shows"}, nil)
}
