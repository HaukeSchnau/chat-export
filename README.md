# chatgpt-exporter

Downloads a public ChatGPT share link and exports the conversation as Markdown.

```sh
nix run github:haukeschnau/chatgpt-exporter -- https://chatgpt.com/share/<id> -o chat.md
```

Flags:

- `-o FILE` write to a file instead of stdout
- `-assets DIR` where to store downloaded images (default `<output>-assets`, or `./assets` for stdout)
- `-no-images` skip image downloads and emit placeholders
- `-thoughts` include reasoning summaries and thinking preambles as blockquotes
- `-json` emit the decoded conversation as JSON
- `-html FILE` parse a share page saved from a browser instead of downloading it

## How it works

Share pages are server-rendered with the full conversation embedded in React
Router's turbo-stream format, so a single request returns every message; the
tool decodes that payload and checks that the linear conversation matches the
path through the message tree. Citation markers in answers become inline links,
and each answer that cites sources gets a sources list.

Images, whether uploaded or generated, are downloaded through the same
anonymous share-scoped file endpoint the web page uses for logged-out
visitors, and linked relative to the Markdown file. Canvas ("writing block") contents are not part of the share
payload and cannot be exported.

## Tests

`testdata/share.html` is a synthetic share page produced by
`testdata/generate-fixture.js` using the real `turbo-stream` encoder, so the
decoder is tested against the genuine wire format without any real chat data.
Regenerate it with `bun add turbo-stream@2 && bun testdata/generate-fixture.js testdata/share.html`.
