package chatgpt

import "testing"

func TestSpliceReferences(t *testing.T) {
	// "\U0001F600" is a surrogate pair, which shifts JavaScript offsets relative
	// to rune offsets. The second marker has no reference and must be dropped.
	text := "\U0001F600 Gerrit is solid.\uE200cite\uE202turn0search7\uE201 More.\uE200cite\uE202turn0search9\uE201"
	refs := []ContentReference{
		{
			Type: "grouped_webpages", MatchedText: "\uE200cite\uE202turn0search7\uE201", StartIdx: 19, EndIdx: 38,
			Items: []RefSource{{Attribution: "gerrit.example", URL: "https://gerrit.example/docs?utm_source=chatgpt.com&x=1"}},
		},
		{
			Type: "sources_footnote", MatchedText: " ", StartIdx: 63, EndIdx: 64,
			Sources: []RefSource{{Title: "Gerrit docs", URL: "https://gerrit.example/docs"}},
		},
	}
	got, sources := spliceReferences(text, refs)
	want := "\U0001F600 Gerrit is solid. ([gerrit.example](https://gerrit.example/docs?x=1)) More."
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
	if len(sources) != 1 || sources[0].Title != "Gerrit docs" {
		t.Errorf("sources = %+v", sources)
	}
}

func TestSpliceReferencesFallsBackToSearch(t *testing.T) {
	marker := "\uE200cite\uE202turn1search2\uE201"
	refs := []ContentReference{{Type: "grouped_webpages", MatchedText: marker, StartIdx: 99, EndIdx: 120, Items: []RefSource{{Attribution: "x", URL: "https://x.test"}}}}
	got, _ := spliceReferences("A"+marker+" B", refs)
	if got != "A ([x](https://x.test)) B" {
		t.Errorf("got %q", got)
	}
}
