package markdown

import "regexp"

// ImageMarkerPrefix labels markers standing in for markdown image references.
// It mirrors MarkerPrefix, which labels mermaid diagrams.
const ImageMarkerPrefix = "GLOWM_IMAGE_"

// MediaKind distinguishes the sources of inline terminal images.
type MediaKind int

const (
	KindMermaid MediaKind = iota
	KindImage
)

// MediaItem is one block that may become an inline terminal image.
//
// For KindMermaid, Source is the diagram text. For KindImage, Source is the
// reference from the markdown (a relative path, absolute path, or URL) and Alt
// is the alt text.
type MediaItem struct {
	Kind   MediaKind
	Marker string
	Source string
	Alt    string
}

// MediaResult is the markdown with media replaced by markers, plus the ordered
// list of media that the markers stand for. Items[i].Marker identifies the
// placeholder for Items[i], so callers can build a parallel slice of rendered
// images indexed the same way.
type MediaResult struct {
	Markdown string
	Items    []MediaItem
}

// Markers returns the marker strings in item order.
func (r MediaResult) Markers() []string {
	markers := make([]string, len(r.Items))
	for i, item := range r.Items {
		markers[i] = item.Marker
	}
	return markers
}

// MermaidBlocks returns the mermaid diagram sources along with the item index
// each one came from, so rendered output can be scattered back into an
// item-indexed slice.
func (r MediaResult) MermaidBlocks() (blocks []string, indices []int) {
	for i, item := range r.Items {
		if item.Kind == KindMermaid {
			blocks = append(blocks, item.Source)
			indices = append(indices, i)
		}
	}
	return blocks, indices
}

// ExtractMedia replaces mermaid blocks and standalone image references with
// markers, returning the rewritten markdown and the media in document order.
func ExtractMedia(md string) (MediaResult, error) {
	return extract(md, extractConfig{useMarkers: true, withImages: true})
}

// imageLineRe matches a line that consists solely of a markdown image
// reference: `![alt](src)`, with an optional title and an optional
// angle-bracketed source.
//
// The pattern is deliberately anchored to the raw line with no allowance for
// leading whitespace. A marker only renders as an image when it survives
// glamour as a line of its own (see termimage.ReplaceMarkerLines), so any
// image nested in a list item, blockquote, table cell, or indented code block
// must be left alone for the text renderer to handle.
var imageLineRe = regexp.MustCompile(`^!\[([^\]]*)\]\(\s*(?:<([^>]*)>|([^\s()]+))(?:\s+(?:"[^"]*"|'[^']*'))?\s*\)$`)

// matchImageLine reports whether line is exactly one image reference and, if
// so, returns its alt text and source.
func matchImageLine(line string) (alt, src string, ok bool) {
	m := imageLineRe.FindStringSubmatch(line)
	if m == nil {
		return "", "", false
	}
	src = m[2] // angle-bracketed form
	if src == "" {
		src = m[3]
	}
	if src == "" {
		return "", "", false
	}
	return m[1], src, true
}
