// Command chatgpt-exporter downloads a public ChatGPT or Claude share link
// and prints the conversation as Markdown.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"strings"

	"github.com/haukeschnau/chatgpt-exporter/internal/chatgpt"
	"github.com/haukeschnau/chatgpt-exporter/internal/claude"
	"github.com/haukeschnau/chatgpt-exporter/internal/convo"
	"github.com/haukeschnau/chatgpt-exporter/internal/markdown"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: chatgpt-exporter [flags] <share-url-or-id>\n\nSupports https://chatgpt.com/share/... and https://claude.ai/share/... links.\n\n")
		flag.PrintDefaults()
	}
	output := flag.String("o", "", "write to this file instead of stdout")
	assetsDir := flag.String("assets", "", "directory for downloaded images (default: <output>-assets, or ./assets when writing to stdout)")
	noImages := flag.Bool("no-images", false, "do not download images; emit placeholders instead")
	asJSON := flag.Bool("json", false, "emit the provider's raw conversation JSON instead of Markdown")
	htmlFile := flag.String("html", "", "read a saved ChatGPT share page instead of downloading (the URL argument becomes optional)")
	thoughts := flag.Bool("thoughts", false, "include reasoning summaries and thinking preambles")
	flag.Parse()
	// Go's flag package stops at the first positional argument; also accept
	// flags after the URL, as in "chatgpt-exporter <url> -o out.md".
	args := flag.Args()
	if len(args) > 1 {
		if err := flag.CommandLine.Parse(args[1:]); err != nil {
			return err
		}
		args = append(args[:1], flag.Args()...)
	}
	if *htmlFile == "" && len(args) != 1 {
		flag.Usage()
		os.Exit(2)
	}

	ctx := context.Background()
	conv, raw, warnings, err := load(ctx, args, *htmlFile)
	if err != nil {
		return err
	}

	var body []byte
	if *asJSON {
		if body, err = json.MarshalIndent(raw, "", "  "); err != nil {
			return err
		}
		body = append(body, '\n')
	} else {
		store := newAssetStore(ctx, *output, *assetsDir)
		opts := markdown.Options{IncludeThoughts: *thoughts}
		if !*noImages {
			opts.ImageSrc = store.src
		}
		body = []byte(markdown.Render(conv, opts))
		warnings = append(warnings, store.warnings...)
	}
	for _, w := range warnings {
		fmt.Fprintln(os.Stderr, "warning:", w)
	}

	if *output == "" {
		_, err = os.Stdout.Write(body)
		return err
	}
	return os.WriteFile(*output, body, 0o644)
}

// load picks the provider from the link and returns the neutral conversation
// alongside the provider's raw payload for -json.
func load(ctx context.Context, args []string, htmlFile string) (*convo.Conversation, any, []string, error) {
	switch {
	case htmlFile != "":
		data, err := os.ReadFile(htmlFile)
		if err != nil {
			return nil, nil, nil, err
		}
		return exportChatGPT(string(data))
	case strings.Contains(args[0], "claude.ai"):
		id, err := claude.ParseShareID(args[0])
		if err != nil {
			return nil, nil, nil, err
		}
		data, err := claude.FetchSnapshot(ctx, id)
		if err != nil {
			return nil, nil, nil, err
		}
		snap, err := claude.Parse(data)
		if err != nil {
			return nil, nil, nil, err
		}
		conv, warnings := snap.Export()
		return conv, snap, warnings, nil
	default:
		id, err := chatgpt.ParseShareID(args[0])
		if err != nil {
			return nil, nil, nil, err
		}
		html, err := chatgpt.FetchHTML(ctx, id)
		if err != nil {
			return nil, nil, nil, err
		}
		return exportChatGPT(html)
	}
}

func exportChatGPT(html string) (*convo.Conversation, any, []string, error) {
	raw, warnings, err := chatgpt.Load(html)
	if err != nil {
		return nil, nil, nil, err
	}
	conv, exportWarnings := raw.Export()
	return conv, raw, append(warnings, exportWarnings...), nil
}

// assetStore downloads each referenced image once into the assets directory
// and hands the renderer a path relative to the Markdown file.
type assetStore struct {
	ctx      context.Context
	dir      string // where files are written
	linkBase string // directory the Markdown file lives in, for relative links
	cache    map[string]string
	warnings []string
}

func newAssetStore(ctx context.Context, output, assetsDir string) *assetStore {
	linkBase := "."
	if output != "" {
		linkBase = filepath.Dir(output)
	}
	if assetsDir == "" {
		if output == "" {
			assetsDir = "assets"
		} else {
			assetsDir = strings.TrimSuffix(output, filepath.Ext(output)) + "-assets"
		}
	}
	return &assetStore{ctx: ctx, dir: assetsDir, linkBase: linkBase, cache: map[string]string{}}
}

func (s *assetStore) src(img convo.ImageRef) string {
	if path, ok := s.cache[img.ID]; ok {
		return path
	}
	path, err := s.download(img)
	if err != nil {
		s.warnings = append(s.warnings, fmt.Sprintf("image %s: %v", img.ID, err))
		path = ""
	}
	s.cache[img.ID] = path
	return path
}

func (s *assetStore) download(img convo.ImageRef) (string, error) {
	data, contentType, err := img.Download(s.ctx)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return "", err
	}
	target := filepath.Join(s.dir, img.ID+extension(contentType))
	if err := os.WriteFile(target, data, 0o644); err != nil {
		return "", err
	}
	rel, err := filepath.Rel(s.linkBase, target)
	if err != nil {
		rel = target
	}
	return filepath.ToSlash(rel), nil
}

func extension(contentType string) string {
	mediaType, _, _ := mime.ParseMediaType(contentType)
	switch mediaType {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	}
	if exts, _ := mime.ExtensionsByType(mediaType); len(exts) > 0 {
		return exts[0]
	}
	return ""
}
