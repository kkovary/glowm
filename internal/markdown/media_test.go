package markdown

import (
	"strings"
	"testing"
)

func TestExtractMediaStandaloneImage(t *testing.T) {
	md := "# Title\n\n![a cat](cat.png)\n\nText\n"
	res, err := ExtractMedia(md)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(res.Items))
	}
	item := res.Items[0]
	if item.Kind != KindImage {
		t.Fatalf("expected KindImage, got %v", item.Kind)
	}
	if item.Source != "cat.png" {
		t.Fatalf("expected source cat.png, got %q", item.Source)
	}
	if item.Alt != "a cat" {
		t.Fatalf("expected alt %q, got %q", "a cat", item.Alt)
	}
	if item.Marker != ImageMarkerPrefix+"0" {
		t.Fatalf("expected marker %q, got %q", ImageMarkerPrefix+"0", item.Marker)
	}
	if strings.Contains(res.Markdown, "cat.png") {
		t.Fatalf("expected image reference replaced, got %q", res.Markdown)
	}
	if !strings.Contains(res.Markdown, item.Marker) {
		t.Fatalf("expected marker in output, got %q", res.Markdown)
	}
}

// A marker only becomes an image if it survives rendering on a line of its
// own, so an image sharing a line or nested in another block must be left for
// the text renderer.
func TestExtractMediaLeavesNonStandaloneImages(t *testing.T) {
	cases := []struct {
		name string
		md   string
	}{
		{"inline with text", "See ![a cat](cat.png) here.\n"},
		{"trailing text", "![a cat](cat.png) is a cat\n"},
		{"list item", "- ![a cat](cat.png)\n"},
		{"blockquote", "> ![a cat](cat.png)\n"},
		{"indented code block", "    ![a cat](cat.png)\n"},
		{"fenced code block", "```\n![a cat](cat.png)\n```\n"},
		{"linked image", "[![a cat](cat.png)](https://example.com)\n"},
		{"reference style", "![a cat][ref]\n\n[ref]: cat.png\n"},
		{"table cell", "| a |\n| --- |\n| ![a cat](cat.png) |\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := ExtractMedia(tc.md)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(res.Items) != 0 {
				t.Fatalf("expected no items, got %d (%+v)", len(res.Items), res.Items)
			}
			if res.Markdown != tc.md {
				t.Fatalf("expected markdown unchanged\n got: %q\nwant: %q", res.Markdown, tc.md)
			}
		})
	}
}

func TestExtractMediaImageSyntaxVariants(t *testing.T) {
	cases := []struct {
		name    string
		md      string
		wantSrc string
		wantAlt string
	}{
		{"plain", "![alt](a.png)\n", "a.png", "alt"},
		{"empty alt", "![](a.png)\n", "a.png", ""},
		{"double quoted title", "![alt](a.png \"the title\")\n", "a.png", "alt"},
		{"single quoted title", "![alt](a.png 'the title')\n", "a.png", "alt"},
		{"angle brackets", "![alt](<my image.png>)\n", "my image.png", "alt"},
		{"padded parens", "![alt]( a.png )\n", "a.png", "alt"},
		{"nested path", "![alt](docs/img/a.png)\n", "docs/img/a.png", "alt"},
		{"absolute path", "![alt](/tmp/a.png)\n", "/tmp/a.png", "alt"},
		{"remote url", "![alt](https://example.com/a.png)\n", "https://example.com/a.png", "alt"},
		{"up to 3 space indent is a fence rule, not an image rule", "  ![alt](a.png)\n", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := ExtractMedia(tc.md)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantSrc == "" {
				if len(res.Items) != 0 {
					t.Fatalf("expected no items, got %+v", res.Items)
				}
				return
			}
			if len(res.Items) != 1 {
				t.Fatalf("expected 1 item, got %d", len(res.Items))
			}
			if res.Items[0].Source != tc.wantSrc {
				t.Fatalf("expected source %q, got %q", tc.wantSrc, res.Items[0].Source)
			}
			if res.Items[0].Alt != tc.wantAlt {
				t.Fatalf("expected alt %q, got %q", tc.wantAlt, res.Items[0].Alt)
			}
		})
	}
}

// An image adjacent to a paragraph must still end up on a line of its own,
// which is what the blank lines around the marker are for.
func TestExtractMediaSeparatesMarkerFromAdjacentText(t *testing.T) {
	md := "Text above\n![alt](a.png)\nText below\n"
	res, err := ExtractMedia(md)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(res.Items))
	}
	marker := res.Items[0].Marker
	lines := strings.Split(res.Markdown, "\n")
	var idx = -1
	for i, line := range lines {
		if line == marker {
			idx = i
			break
		}
	}
	if idx < 0 {
		t.Fatalf("expected marker on a line of its own, got %q", res.Markdown)
	}
	if lines[idx-1] != "" {
		t.Fatalf("expected blank line before marker, got %q", lines[idx-1])
	}
	if lines[idx+1] != "" {
		t.Fatalf("expected blank line after marker, got %q", lines[idx+1])
	}
}

func TestExtractMediaMixedOrdering(t *testing.T) {
	md := "![one](one.png)\n\n```mermaid\nA-->B\n```\n\n![two](two.png)\n\n```mermaid\nC-->D\n```\n"
	res, err := ExtractMedia(md)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Items) != 4 {
		t.Fatalf("expected 4 items, got %d", len(res.Items))
	}
	wantKinds := []MediaKind{KindImage, KindMermaid, KindImage, KindMermaid}
	for i, want := range wantKinds {
		if res.Items[i].Kind != want {
			t.Fatalf("item %d: expected kind %v, got %v", i, want, res.Items[i].Kind)
		}
	}

	// Markers carry the item index, so images and diagrams share one sequence.
	wantMarkers := []string{
		ImageMarkerPrefix + "0",
		MarkerPrefix + "1",
		ImageMarkerPrefix + "2",
		MarkerPrefix + "3",
	}
	for i, want := range wantMarkers {
		if res.Items[i].Marker != want {
			t.Fatalf("item %d: expected marker %q, got %q", i, want, res.Items[i].Marker)
		}
	}
	if got := res.Markers(); len(got) != 4 || got[2] != wantMarkers[2] {
		t.Fatalf("unexpected markers: %v", got)
	}

	// MermaidBlocks must report the item index each diagram came from so
	// rendered output can be scattered back into an item-indexed slice.
	blocks, indices := res.MermaidBlocks()
	if len(blocks) != 2 {
		t.Fatalf("expected 2 mermaid blocks, got %d", len(blocks))
	}
	if blocks[0] != "A-->B" || blocks[1] != "C-->D" {
		t.Fatalf("unexpected blocks: %q", blocks)
	}
	if len(indices) != 2 || indices[0] != 1 || indices[1] != 3 {
		t.Fatalf("expected indices [1 3], got %v", indices)
	}
}

// The unclosed-fence rollback drops the pending diagram; an image extracted
// beforehand must survive it with its marker intact.
func TestExtractMediaUnclosedFenceAfterImage(t *testing.T) {
	md := "![alt](a.png)\n\n```mermaid\nA-->B\n"
	res, err := ExtractMedia(md)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Items) != 1 {
		t.Fatalf("expected 1 item, got %d (%+v)", len(res.Items), res.Items)
	}
	if res.Items[0].Kind != KindImage {
		t.Fatalf("expected the image to survive, got %v", res.Items[0].Kind)
	}
	if !strings.Contains(res.Markdown, "```mermaid") {
		t.Fatalf("expected unclosed fence restored, got %q", res.Markdown)
	}
	if !strings.Contains(res.Markdown, "A-->B") {
		t.Fatalf("expected unclosed fence content restored, got %q", res.Markdown)
	}
}

// Images are only extracted on the media path; the mermaid-only entry points
// must leave them for the text renderer.
func TestExtractMermaidIgnoresImages(t *testing.T) {
	md := "![alt](a.png)\n\n```mermaid\nA-->B\n```\n"
	res, err := ExtractMermaidWithMarkers(md)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Markers) != 1 {
		t.Fatalf("expected 1 marker, got %d", len(res.Markers))
	}
	if res.Markers[0] != MarkerPrefix+"0" {
		t.Fatalf("expected mermaid marker numbered from 0, got %q", res.Markers[0])
	}
	if !strings.Contains(res.Markdown, "![alt](a.png)") {
		t.Fatalf("expected image reference untouched, got %q", res.Markdown)
	}
}
