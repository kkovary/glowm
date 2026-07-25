package termimage

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
)

func TestReplaceMarkersWithImages_Basic(t *testing.T) {
	markers := []string{"GLOWM_MERMAID_0"}
	images := [][]byte{[]byte("png")}
	output := "before\nGLOWM_MERMAID_0\nafter"

	result := ReplaceMarkersWithImages(output, markers, images, nil, FormatIterm2, 80)
	if strings.Contains(result, "GLOWM_MERMAID_0") {
		t.Fatal("expected marker to be replaced")
	}
	if !strings.Contains(result, "\x1b]1337;File=") {
		t.Fatal("expected iTerm2 image sequence")
	}
	if !strings.Contains(result, "before") || !strings.Contains(result, "after") {
		t.Fatal("expected surrounding text to be preserved")
	}
}

func TestReplaceMarkersWithImages_Empty(t *testing.T) {
	result := ReplaceMarkersWithImages("hello", nil, nil, nil, FormatIterm2, 80)
	if result != "hello" {
		t.Fatalf("expected unchanged output, got %q", result)
	}
}

func TestReplaceMarkersWithImages_MoreMarkersThanImages(t *testing.T) {
	markers := []string{"GLOWM_MERMAID_0", "GLOWM_MERMAID_1"}
	images := [][]byte{[]byte("png")}
	output := "GLOWM_MERMAID_0\nGLOWM_MERMAID_1"

	result := ReplaceMarkersWithImages(output, markers, images, nil, FormatIterm2, 80)
	// First marker should be replaced, second should remain.
	if strings.Contains(result, "GLOWM_MERMAID_0") {
		t.Fatal("expected first marker to be replaced")
	}
	if !strings.Contains(result, "GLOWM_MERMAID_1") {
		t.Fatal("expected second marker to remain (no matching image)")
	}
}

func TestReplaceMarkersWithImages_FormatNone(t *testing.T) {
	markers := []string{"GLOWM_MERMAID_0"}
	images := [][]byte{[]byte("png")}
	output := "GLOWM_MERMAID_0"

	result := ReplaceMarkersWithImages(output, markers, images, nil, FormatNone, 80)
	if result != output {
		t.Fatalf("expected unchanged output for FormatNone, got %q", result)
	}
}

func TestReplaceMarkersWithImagesForPager_AddsImageRowMarker(t *testing.T) {
	markers := []string{"GLOWM_MERMAID_0"}
	images := [][]byte{pngFixture(t, 100, 100)}
	output := "before\nGLOWM_MERMAID_0\nafter"

	result := ReplaceMarkersWithImagesForPager(output, markers, images, nil, FormatKitty, 80)

	if strings.Contains(result, "GLOWM_MERMAID_0") {
		t.Fatal("expected marker to be replaced")
	}
	if !strings.Contains(result, "\x1b]1337;glowm-rows=40\x07") {
		t.Fatalf("expected pager row marker, got %q", result)
	}
	if got := strings.Count(result, "\n"); got != 2 {
		t.Fatalf("expected pager replacement to preserve line count, got %d newlines", got)
	}
}

func TestReplaceMarkersWithImages_DoesNotPadImageRows(t *testing.T) {
	markers := []string{"GLOWM_MERMAID_0"}
	images := [][]byte{pngFixture(t, 100, 100)}
	output := "before\nGLOWM_MERMAID_0\nafter"

	result := ReplaceMarkersWithImages(output, markers, images, nil, FormatKitty, 80)

	if got := strings.Count(result, "\n"); got != 2 {
		t.Fatalf("expected normal replacement to preserve line count, got %d newlines", got)
	}
}

func TestImageRows_Fallbacks(t *testing.T) {
	if got := imageRows(pngFixture(t, 100, 100), 0); got != 1 {
		t.Fatalf("imageRows(width=0) = %d, want 1", got)
	}
	if got := imageRows([]byte("not png"), 80); got != 1 {
		t.Fatalf("imageRows(invalid png) = %d, want 1", got)
	}
	if got := imageRows(pngFixture(t, 100, 1), 1); got != 1 {
		t.Fatalf("imageRows(tiny image) = %d, want minimum 1", got)
	}
}

func TestStripANSI_NoEscapes(t *testing.T) {
	input := "hello world"
	if got := stripANSI(input); got != input {
		t.Fatalf("expected %q, got %q", input, got)
	}
}

func TestStripANSI_CSI(t *testing.T) {
	input := "\x1b[31mred\x1b[0m"
	got := stripANSI(input)
	if got != "red" {
		t.Fatalf("expected 'red', got %q", got)
	}
}

func TestStripANSI_OSC(t *testing.T) {
	input := "\x1b]8;;http://example.com\x07link\x1b]8;;\x07"
	got := stripANSI(input)
	if got != "link" {
		t.Fatalf("expected 'link', got %q", got)
	}
}

func TestStripANSI_APC(t *testing.T) {
	input := "\x1b_Gf=100,a=T,m=0;AAAA\x1b\\"
	got := stripANSI(input)
	if got != "" {
		t.Fatalf("expected empty string for APC sequence, got %q", got)
	}
}

func TestStripANSI_TruncatedEscape(t *testing.T) {
	input := "text\x1b"
	got := stripANSI(input)
	if got != "text" {
		t.Fatalf("expected 'text', got %q", got)
	}
}

func TestStripANSI_MalformedCSIBeforeTerminator(t *testing.T) {
	// A new ESC arrives inside a CSI sequence before its letter terminator.
	// The first (malformed) sequence is abandoned and the second is parsed,
	// so only the visible text survives.
	input := "\x1b[31\x1b[0mvisible"
	got := stripANSI(input)
	if got != "visible" {
		t.Fatalf("expected 'visible', got %q", got)
	}
}

func TestStripANSI_Mixed(t *testing.T) {
	input := "\x1b[1mbold\x1b[0m and \x1b]8;;url\x07link\x1b]8;;\x07"
	got := stripANSI(input)
	if got != "bold and link" {
		t.Fatalf("expected 'bold and link', got %q", got)
	}
}

func TestReplaceMarkersWithImages_ANSIWrappedMarker(t *testing.T) {
	markers := []string{"GLOWM_MERMAID_0"}
	images := [][]byte{[]byte("png")}
	// Marker wrapped in ANSI color codes (as glamour might do).
	output := "\x1b[1mGLOWM_MERMAID_0\x1b[0m"

	result := ReplaceMarkersWithImages(output, markers, images, nil, FormatIterm2, 80)
	if strings.Contains(result, "GLOWM_MERMAID_0") {
		t.Fatal("expected ANSI-wrapped marker to be replaced")
	}
}

func TestReplaceMarkersWithImages_KittyFormat(t *testing.T) {
	markers := []string{"GLOWM_MERMAID_0"}
	images := [][]byte{[]byte("png")}
	output := "GLOWM_MERMAID_0"

	result := ReplaceMarkersWithImages(output, markers, images, nil, FormatKitty, 80)
	if strings.Contains(result, "GLOWM_MERMAID_0") {
		t.Fatal("expected marker to be replaced")
	}
	if !strings.Contains(result, "\x1b_G") {
		t.Fatal("expected Kitty APC sequence")
	}
}

func TestStripANSI_TwoByteEscape(t *testing.T) {
	// ESC(B is a common "select character set" two-byte sequence.
	input := "text\x1b(Bmore"
	got := stripANSI(input)
	if got != "textmore" {
		t.Fatalf("expected 'textmore', got %q", got)
	}
}

func TestStripANSI_MalformedCSI(t *testing.T) {
	// CSI with no terminating letter — should not consume following content.
	input := "\x1b[999text"
	got := stripANSI(input)
	// The 't' in 'text' terminates the CSI, so 'ext' remains.
	if got != "ext" {
		t.Fatalf("expected 'ext', got %q", got)
	}
}

func TestStripANSI_DCS(t *testing.T) {
	// DCS sequence: ESC P ... ESC \
	input := "\x1bPsome data\x1b\\visible"
	got := stripANSI(input)
	if got != "visible" {
		t.Fatalf("expected 'visible', got %q", got)
	}
}

func pngFixture(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	img.Set(0, 0, color.White)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestReplaceMarkersWithImages_SkipsEmptyImage(t *testing.T) {
	// An entry with no image data means rendering failed for that marker. The
	// marker has to survive so the caller can substitute text for it, rather
	// than becoming an image escape with an empty payload.
	markers := []string{"GLOWM_IMAGE_0", "GLOWM_IMAGE_1"}
	images := [][]byte{nil, []byte("png")}
	output := "GLOWM_IMAGE_0\nGLOWM_IMAGE_1"

	result := ReplaceMarkersWithImages(output, markers, images, nil, FormatIterm2, 80)
	if !strings.Contains(result, "GLOWM_IMAGE_0") {
		t.Fatal("expected the marker without an image to be left in place")
	}
	if strings.Contains(result, "GLOWM_IMAGE_1") {
		t.Fatal("expected the marker with an image to be replaced")
	}
	if got := strings.Count(result, "\x1b]1337;File="); got != 1 {
		t.Fatalf("expected exactly 1 image sequence, got %d", got)
	}
}

func TestReplaceMarkerLines(t *testing.T) {
	output := "before\n  \x1b[38;5;252mMARK_A\x1b[m  \nMARK_B inline\nafter"
	result := ReplaceMarkerLines(output, map[string]string{
		"MARK_A": "replaced-a",
		"MARK_B": "replaced-b",
	})
	if !strings.Contains(result, "replaced-a") {
		t.Fatal("expected a styled, indented marker line to be replaced")
	}
	// Only whole lines are replaced: a marker sharing a line is left alone.
	if !strings.Contains(result, "MARK_B inline") {
		t.Fatal("expected a marker sharing a line to be left alone")
	}
	if !strings.Contains(result, "before") || !strings.Contains(result, "after") {
		t.Fatal("expected surrounding lines preserved")
	}
}

func TestReplaceMarkerLines_Empty(t *testing.T) {
	if got := ReplaceMarkerLines("hello", nil); got != "hello" {
		t.Fatalf("expected unchanged output, got %q", got)
	}
}

func TestStripANSI_Exported(t *testing.T) {
	if got := StripANSI("\x1b[38;5;252mtext\x1b[m"); got != "text" {
		t.Fatalf("expected %q, got %q", "text", got)
	}
}

func TestNaturalWidthCells(t *testing.T) {
	// 900px at 9px per cell is 100 cells.
	if got := NaturalWidthCells(pngFixture(t, 900, 100)); got != 100 {
		t.Fatalf("expected 100 cells, got %d", got)
	}
	// Partial cells round up rather than truncating the image.
	if got := NaturalWidthCells(pngFixture(t, 100, 100)); got != 12 {
		t.Fatalf("expected 12 cells, got %d", got)
	}
	// A sub-cell image still occupies one cell.
	if got := NaturalWidthCells(pngFixture(t, 1, 1)); got != 1 {
		t.Fatalf("expected 1 cell, got %d", got)
	}
	// An unmeasurable image reports no cap.
	if got := NaturalWidthCells([]byte("not png")); got != 0 {
		t.Fatalf("expected 0 for undecodable input, got %d", got)
	}
}

func TestReplaceMarkersWithImages_CapsDisplayWidth(t *testing.T) {
	markers := []string{"GLOWM_IMAGE_0"}
	images := [][]byte{pngFixture(t, 90, 90)}
	output := "GLOWM_IMAGE_0"

	// A 90px image is 10 cells wide, so it must not be stretched to 80.
	result := ReplaceMarkersWithImages(output, markers, images, []int{10}, FormatKitty, 80)
	if !strings.Contains(result, "c=10,") {
		t.Fatalf("expected the capped width, got %q", firstBytes(result))
	}
	if strings.Contains(result, "c=80,") {
		t.Fatalf("expected the image not to fill the terminal, got %q", firstBytes(result))
	}
}

func TestReplaceMarkersWithImages_CapWiderThanTerminalIsIgnored(t *testing.T) {
	markers := []string{"GLOWM_IMAGE_0"}
	images := [][]byte{pngFixture(t, 4000, 100)}
	output := "GLOWM_IMAGE_0"

	// A cap above the available width must not widen the image past it.
	result := ReplaceMarkersWithImages(output, markers, images, []int{445}, FormatKitty, 80)
	if !strings.Contains(result, "c=80,") {
		t.Fatalf("expected the available width, got %q", firstBytes(result))
	}
}

func TestReplaceMarkersWithImages_ZeroAndMissingCaps(t *testing.T) {
	markers := []string{"GLOWM_IMAGE_0", "GLOWM_IMAGE_1"}
	images := [][]byte{pngFixture(t, 90, 90), pngFixture(t, 90, 90)}
	output := "GLOWM_IMAGE_0\nGLOWM_IMAGE_1"

	// A zero entry and a short slice both mean "use the full width", which is
	// what a diagram rasterized to the display width wants.
	result := ReplaceMarkersWithImages(output, markers, images, []int{0}, FormatKitty, 80)
	if got := strings.Count(result, "c=80,"); got != 2 {
		t.Fatalf("expected both images at full width, got %d (%q)", got, firstBytes(result))
	}
}

// The pager's row count has to follow the capped width, or it will scroll past
// the wrong number of rows.
func TestReplaceMarkersWithImagesForPager_RowsFollowCappedWidth(t *testing.T) {
	markers := []string{"GLOWM_IMAGE_0"}
	images := [][]byte{pngFixture(t, 90, 90)}
	output := "GLOWM_IMAGE_0"

	result := ReplaceMarkersWithImagesForPager(output, markers, images, []int{10}, FormatKitty, 80)
	// Square image at 10 cells wide: 10 / 2 = 5 rows.
	if !strings.Contains(result, "glowm-rows=5\x07") {
		t.Fatalf("expected 5 rows for the capped width, got %q", firstBytes(result))
	}
}

// firstBytes trims an image escape sequence down to something readable in a
// test failure message.
func firstBytes(s string) string {
	if len(s) > 120 {
		return s[:120] + "..."
	}
	return s
}
