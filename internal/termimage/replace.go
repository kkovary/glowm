package termimage

import (
	"bytes"
	"image"
	_ "image/png"
	"math"
	"strconv"
	"strings"
)

// ReplaceMarkersWithImages substitutes an inline image for every line that
// consists solely of one of the markers, with images[i] belonging to
// markers[i].
//
// widthCells is the width available for display. maxWidths optionally caps an
// individual image's display width so that a small image is not upscaled to
// fill the terminal; a nil or short slice, or a zero entry, gives that image
// the full available width.
func ReplaceMarkersWithImages(output string, markers []string, images [][]byte, maxWidths []int, format Format, widthCells int) string {
	return replaceMarkersWithImages(output, markers, images, maxWidths, format, widthCells, false)
}

// ReplaceMarkersWithImagesForPager behaves like ReplaceMarkersWithImages but
// prefixes each image with the row count it occupies, which the pager needs in
// order to scroll past it.
func ReplaceMarkersWithImagesForPager(output string, markers []string, images [][]byte, maxWidths []int, format Format, widthCells int) string {
	return replaceMarkersWithImages(output, markers, images, maxWidths, format, widthCells, true)
}

func replaceMarkersWithImages(output string, markers []string, images [][]byte, maxWidths []int, format Format, widthCells int, padToImageRows bool) string {
	if len(markers) == 0 || len(images) == 0 {
		return output
	}
	lookup := make(map[string]string, len(markers))
	for i, marker := range markers {
		if i >= len(images) {
			break
		}
		if len(images[i]) == 0 {
			// No image for this marker: leave the marker in place so the
			// caller can substitute text for it.
			continue
		}
		cells := widthCells
		if i < len(maxWidths) {
			cells = DisplayWidthCells(widthCells, maxWidths[i])
		}
		img := EncodeWithWidth(format, images[i], cells)
		if img == "" {
			continue
		}
		if padToImageRows {
			img = pagerRowsMarker(imageRows(images[i], cells)) + img
		}
		lookup[marker] = img
	}

	return ReplaceMarkerLines(output, lookup)
}

// ReplaceMarkerLines substitutes every line of output that consists solely of
// a key in replacements with that key's value. Lines are compared with ANSI
// escapes stripped and surrounding whitespace trimmed, so a marker still
// matches after the renderer has styled and indented it.
func ReplaceMarkerLines(output string, replacements map[string]string) string {
	if len(replacements) == 0 {
		return output
	}
	lines := strings.Split(output, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(stripANSI(line))
		if replacement, ok := replacements[trimmed]; ok {
			lines[i] = replacement
		}
	}
	return strings.Join(lines, "\n")
}

func pagerRowsMarker(rows int) string {
	return "\x1b]1337;glowm-rows=" + strconv.Itoa(rows) + "\x07"
}

// pixelsPerCell is the assumed width of a terminal cell. It matches the
// viewport width the mermaid renderer rasterizes at, so a diagram rendered for
// a given cell width reports that same width back here.
const pixelsPerCell = 9.0

// DisplayWidthCells returns the width in cells an image should occupy: the
// available width, narrowed to maxWidth when that is a smaller positive value.
// A zero or negative maxWidth means no cap, so the image takes the full width.
func DisplayWidthCells(widthCells, maxWidth int) int {
	if maxWidth > 0 && maxWidth < widthCells {
		return maxWidth
	}
	return widthCells
}

// NaturalWidthCells returns the width in terminal cells that png occupies at
// its own resolution, or 0 if the image cannot be measured. Use it to cap an
// image's display width so that a small image is shown at its natural size
// instead of being upscaled to fill the terminal.
func NaturalWidthCells(png []byte) int {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(png))
	if err != nil || cfg.Width <= 0 {
		return 0
	}
	cells := int(math.Ceil(float64(cfg.Width) / pixelsPerCell))
	if cells < 1 {
		return 1
	}
	return cells
}

func imageRows(png []byte, widthCells int) int {
	if widthCells <= 0 {
		return 1
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(png))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 {
		return 1
	}
	const cellAspect = 2.0 // terminal cells are roughly twice as tall as wide.
	rows := int(math.Ceil((float64(cfg.Height) / float64(cfg.Width)) * float64(widthCells) / cellAspect))
	if rows < 1 {
		return 1
	}
	return rows
}

// StripANSI removes ANSI escape sequences from s, leaving the printable text.
func StripANSI(s string) string {
	return stripANSI(s)
}

func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c != 0x1b {
			b.WriteByte(c)
			continue
		}
		if i+1 >= len(s) {
			continue
		}
		next := s[i+1]
		if next == '[' {
			// CSI sequence: ESC [ ... <letter>
			i += 2
			for i < len(s) {
				ch := s[i]
				if (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') {
					break
				}
				if ch == 0x1b {
					// Malformed: new escape before terminator.
					i--
					break
				}
				i++
			}
			continue
		}
		if next == ']' {
			// OSC sequence: ESC ] ... (BEL | ESC \)
			i += 2
			for i < len(s) {
				if s[i] == 0x07 {
					break
				}
				if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '\\' {
					i++
					break
				}
				i++
			}
			continue
		}
		if next == '_' || next == 'P' || next == '^' {
			// APC / DCS / PM sequence: ESC <type> ... ESC \
			i += 2
			for i < len(s) {
				if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '\\' {
					i++
					break
				}
				i++
			}
			continue
		}
		// Two-byte or three-byte escape sequences.
		// ESC( ESC) ESC* ESC+ are followed by a character set designator byte.
		if next == '(' || next == ')' || next == '*' || next == '+' {
			i += 2 // skip ESC + type + designator
			continue
		}
		// Other two-byte: ESC + single byte (e.g. ESC 7, ESC 8, ESC =).
		i++
		continue
	}
	return b.String()
}
