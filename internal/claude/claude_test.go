package claude

import (
	"os"
	"strings"
	"testing"

	"github.com/haukeschnau/chatgpt-exporter/internal/convo"
)

func fixture(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("../../testdata/claude-snapshot.json")
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestParseAndExport(t *testing.T) {
	snap, err := Parse(fixture(t))
	if err != nil {
		t.Fatal(err)
	}
	conv, warnings := snap.Export()
	if conv.Title != "Fixture: Planning a Picnic 🧺" || conv.Provider != "claude" || conv.AssistantName != "Claude" {
		t.Errorf("header = %+v", conv)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "1 attachment") {
		t.Errorf("warnings = %v", warnings)
	}
	if got := len(conv.Turns); got != 4 {
		t.Fatalf("%d turns, want 4", got)
	}
	kinds := func(turn int) []convo.Kind {
		var out []convo.Kind
		for _, b := range conv.Turns[turn].Blocks {
			out = append(out, b.Kind)
		}
		return out
	}
	if k := kinds(0); len(k) != 2 || k[0] != convo.KindText || !strings.Contains(conv.Turns[0].Blocks[0].Text, "1 attachment not included") {
		t.Errorf("user turn blocks = %v", k)
	}
	if k := kinds(1); len(k) != 4 || k[0] != convo.KindThought || k[2] != convo.KindText || !strings.Contains(conv.Turns[1].Blocks[2].Text, "[Picnic Checklist](https://claude.ai/code/artifact/") {
		t.Errorf("assistant turn blocks = %v", k)
	}
	if conv.Turns[1].Blocks[0].Thoughts[0].Summary != "Looking at the park" {
		t.Errorf("thought summary = %q", conv.Turns[1].Blocks[0].Thoughts[0].Summary)
	}
	if conv.Turns[2].Blocks[0].Text != "Thanks! Anything else?" {
		t.Error("legacy text field not used when content is empty")
	}
	if conv.Created.Format("2006-01-02T15:04:05Z") != "2024-01-01T09:00:00Z" || conv.Shared.Format("2006-01-02") != "2024-01-02" {
		t.Errorf("times = %v %v %v", conv.Created, conv.Updated, conv.Shared)
	}
}

func TestParseRejectsBrokenChain(t *testing.T) {
	broken := strings.Replace(string(fixture(t)), `"parent_message_uuid": "msg-0002"`, `"parent_message_uuid": "msg-0001"`, 1)
	if _, err := Parse([]byte(broken)); err == nil {
		t.Error("expected error for message that does not continue the chain")
	}
}

func TestParseShareID(t *testing.T) {
	id, err := ParseShareID("https://claude.ai/share/0d7b072f-5527-4280-b8db-dd896db77032")
	if err != nil || id != "0d7b072f-5527-4280-b8db-dd896db77032" {
		t.Errorf("got %q, %v", id, err)
	}
}
