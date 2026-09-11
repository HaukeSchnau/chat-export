// Command chatgpt-exporter downloads a public ChatGPT share link and prints
// the conversation as Markdown.
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

	"github.com/haukeschnau/chatgpt-exporter/internal/markdown"
	"github.com/haukeschnau/chatgpt-exporter/internal/share"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: chatgpt-exporter [flags] <share-url-or-id>\n\n")
		flag.PrintDefaults()
	}
	output := flag.String("o", "", "write to this file instead of stdout")
	assetsDir := flag.String("assets", "", "directory for downloaded images (default: <output>-assets, or ./assets when writing to stdout)")
	noImages := flag.Bool("no-images", false, "do not download images; emit placeholders instead")
	asJSON := flag.Bool("json", false, "emit the raw conversation JSON instead of Markdown")
	htmlFile := flag.String("html", "", "read a saved share page instead of downloading (the URL argument becomes optional)")
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

	ctx := context.Background()
	var html string
	switch {
	case *htmlFile != "":
		data, err := os.ReadFile(*htmlFile)
		if err != nil {
			return err
		}
		html = string(data)
	case len(args) == 1:
		id, err := share.ParseShareID(args[0])
		if err != nil {
			return err
		}
		if html, err = share.FetchHTML(ctx, id); err != nil {
			return err
		}
	default:
		flag.Usage()
		os.Exit(2)
	}

	conv, warnings, err := share.Load(html)
	if err != nil {
		return err
	}

	var body []byte
	if *asJSON {
		if body, err = json.MarshalIndent(conv, "", "  "); err != nil {
			return err
		}
		body = append(body, '\n')
	} else {
		opts := markdown.Options{
			SourceURL:       share.ShareURL(conv.ConversationID),
			IncludeThoughts: *thoughts,
		}
		store := newAssetStore(ctx, conv.ConversationID, *output, *assetsDir)
		if !*noImages {
			opts.ImageSrc = store.src
		}
		doc, renderWarnings := markdown.Render(conv, opts)
		warnings = append(append(warnings, renderWarnings...), store.warnings...)
		body = []byte(doc)
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

// assetStore downloads each referenced image once into the assets directory
// and hands the renderer a path relative to the Markdown file.
type assetStore struct {
	ctx      context.Context
	shareID  string
	dir      string // where files are written
	linkBase string // directory the Markdown file lives in, for relative links
	cache    map[string]string
	warnings []string
}

func newAssetStore(ctx context.Context, shareID, output, assetsDir string) *assetStore {
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
	return &assetStore{ctx: ctx, shareID: shareID, dir: assetsDir, linkBase: linkBase, cache: map[string]string{}}
}

func (s *assetStore) src(img share.ImagePart) string {
	if path, ok := s.cache[img.FileID]; ok {
		return path
	}
	path, err := s.download(img.FileID)
	if err != nil {
		s.warnings = append(s.warnings, fmt.Sprintf("image %s: %v", img.FileID, err))
		path = ""
	}
	s.cache[img.FileID] = path
	return path
}

func (s *assetStore) download(fileID string) (string, error) {
	data, contentType, err := share.DownloadFile(s.ctx, s.shareID, fileID)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return "", err
	}
	target := filepath.Join(s.dir, fileID+extension(contentType))
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
