package chatgpt

import (
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"unicode/utf16"
)

// spliceReferences replaces citation markers in text with Markdown links and
// collects the sources listed by a trailing sources_footnote reference.
//
// Reference offsets are UTF-16 code unit indexes because they were computed in
// JavaScript, so the text is converted before slicing. Each marker is verified
// against matched_text; on mismatch the marker is located by searching for
// matched_text instead, and if that fails it is left for the final cleanup.
func spliceReferences(text string, refs []ContentReference) (string, []RefSource) {
	units := utf16.Encode([]rune(text))
	var sources []RefSource

	type edit struct {
		start, end  int
		replacement string
	}
	var edits []edit
	for _, ref := range refs {
		if ref.Type == "sources_footnote" {
			// Anchored past the end of the text; it is a list, not a marker.
			sources = append(sources, ref.Sources...)
			continue
		}
		start, end := locate(units, ref)
		if start < 0 {
			continue
		}
		var replacement string
		switch ref.Type {
		case "grouped_webpages":
			replacement = inlineCitation(ref)
		case "hidden":
		default:
			replacement = ref.Alt
		}
		edits = append(edits, edit{start, end, replacement})
	}
	sort.Slice(edits, func(i, j int) bool { return edits[i].start < edits[j].start })

	var b strings.Builder
	pos := 0
	for _, e := range edits {
		if e.start < pos {
			continue // overlapping reference, already consumed
		}
		b.WriteString(string(utf16.Decode(units[pos:e.start])))
		b.WriteString(e.replacement)
		pos = e.end
	}
	b.WriteString(string(utf16.Decode(units[pos:])))
	return stripMarkers(b.String()), sources
}

// locate returns the UTF-16 range of a reference marker, or -1 if not found.
func locate(units []uint16, ref ContentReference) (int, int) {
	matched := utf16.Encode([]rune(ref.MatchedText))
	if ref.StartIdx >= 0 && ref.EndIdx <= len(units) && ref.StartIdx <= ref.EndIdx {
		if len(matched) == 0 || equalUnits(units[ref.StartIdx:ref.EndIdx], matched) {
			return ref.StartIdx, ref.EndIdx
		}
	}
	if len(matched) > 0 && strings.TrimSpace(ref.MatchedText) != "" {
		if i := indexUnits(units, matched); i >= 0 {
			return i, i + len(matched)
		}
	}
	return -1, -1
}

// inlineCitation renders a grouped_webpages reference the way ChatGPT's own
// alt text does: "([site](url), [site2](url2))".
func inlineCitation(ref ContentReference) string {
	var links []string
	seen := map[string]bool{}
	for _, item := range ref.Items {
		u := cleanURL(item.URL)
		if u == "" || seen[u] {
			continue
		}
		seen[u] = true
		label := item.Attribution
		if label == "" {
			label = item.Title
		}
		links = append(links, fmt.Sprintf("[%s](%s)", label, u))
	}
	if len(links) == 0 {
		return ref.Alt
	}
	return " (" + strings.Join(links, ", ") + ")"
}

// cleanURL drops the utm_source=chatgpt.com tracking parameter ChatGPT appends.
func cleanURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	q := u.Query()
	if q.Get("utm_source") == "chatgpt.com" {
		q.Del("utm_source")
		u.RawQuery = q.Encode()
	}
	return u.String()
}

// unmatchedMarker matches a whole citation marker, delimited by U+E200 and
// U+E201, that no content reference claimed. The web UI hides these too.
var unmatchedMarker = regexp.MustCompile("\uE200[^\uE201]*\uE201")

// stripMarkers removes unreferenced markers and any leftover private-use
// delimiter characters.
func stripMarkers(s string) string {
	s = unmatchedMarker.ReplaceAllString(s, "")
	return strings.Map(func(r rune) rune {
		if r >= 0xE000 && r <= 0xF8FF {
			return -1
		}
		return r
	}, s)
}

func equalUnits(a, b []uint16) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func indexUnits(haystack, needle []uint16) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if equalUnits(haystack[i:i+len(needle)], needle) {
			return i
		}
	}
	return -1
}
