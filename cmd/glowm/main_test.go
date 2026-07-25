package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/atani/glowm/internal/markdown"
	"github.com/atani/glowm/internal/pager"
	"github.com/atani/glowm/internal/termimage"
)

func TestParseFlags_Defaults(t *testing.T) {
	opts, err := parseFlags(nil)
	if err != nil {
		t.Fatalf("parseFlags(nil) error: %v", err)
	}
	if opts.width != 0 || opts.style != "auto" || opts.usePager || opts.noPager || opts.pdf || opts.showVersion || opts.showLinkURLs {
		t.Errorf("unexpected defaults: %+v", opts)
	}
}

func TestParseFlags_ShowLinkURLs(t *testing.T) {
	opts, err := parseFlags([]string{"-show-link-urls", "file.md"})
	if err != nil {
		t.Fatalf("parseFlags error: %v", err)
	}
	if !opts.showLinkURLs {
		t.Errorf("showLinkURLs = false, want true")
	}
}

func TestParseFlags_AllSet(t *testing.T) {
	opts, err := parseFlags([]string{"-w", "100", "-s", "dark", "-p", "-no-pager", "file.md"})
	if err != nil {
		t.Fatalf("parseFlags error: %v", err)
	}
	if opts.width != 100 {
		t.Errorf("width = %d, want 100", opts.width)
	}
	if opts.style != "dark" {
		t.Errorf("style = %q, want dark", opts.style)
	}
	if !opts.usePager || !opts.noPager {
		t.Errorf("pager flags not set: %+v", opts)
	}
	if len(opts.positional) != 1 || opts.positional[0] != "file.md" {
		t.Errorf("positional = %v, want [file.md]", opts.positional)
	}
}

func TestParseFlags_Invalid(t *testing.T) {
	_, err := parseFlags([]string{"-nonexistent-flag"})
	if err == nil {
		t.Fatal("expected error for unknown flag")
	}
}

func TestRun_Version(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := run([]string{"-version"}, &out, &errBuf)
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "glowm") {
		t.Errorf("version output missing 'glowm': %q", out.String())
	}
}

func TestRun_InvalidFlag(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := run([]string{"-bogus"}, &out, &errBuf)
	if code != 2 {
		t.Errorf("exit code = %d, want 2 for invalid flag", code)
	}
}

func TestRun_RendersFileToStdout(t *testing.T) {
	// Non-TTY path: a markdown file is rendered and written to the provided
	// stdout writer. Under `go test` stdout is not a terminal, so this
	// exercises the common text-only branch end to end.
	dir := t.TempDir()
	mdPath := filepath.Join(dir, "doc.md")
	if err := os.WriteFile(mdPath, []byte("# Heading\n\nbody paragraph\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errBuf bytes.Buffer
	code := run([]string{"-s", "notty", mdPath}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, errBuf.String())
	}
	if !strings.Contains(out.String(), "Heading") {
		t.Errorf("output missing heading: %q", out.String())
	}
	if !strings.Contains(out.String(), "body paragraph") {
		t.Errorf("output missing body: %q", out.String())
	}
}

func TestRun_MissingFile(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := run([]string{filepath.Join(t.TempDir(), "does-not-exist.md")}, &out, &errBuf)
	if code != 1 {
		t.Errorf("exit code = %d, want 1 for missing file", code)
	}
	if errBuf.Len() == 0 {
		t.Error("expected an error message on stderr")
	}
}

func TestRun_NoInput(t *testing.T) {
	// No positional args and stdin is not a pipe (interactive test env) ->
	// input.Read returns ErrNoInput -> exit 1. Skip when stdin happens to be
	// a pipe (e.g. CI feeding stdin) to avoid blocking on a read.
	stat, err := os.Stdin.Stat()
	if err == nil && (stat.Mode()&os.ModeCharDevice) == 0 {
		t.Skip("stdin is a pipe in this environment; skipping no-input case")
	}
	var out, errBuf bytes.Buffer
	code := run(nil, &out, &errBuf)
	if code != 1 {
		t.Errorf("exit code = %d, want 1 when no input", code)
	}
}

func TestFail(t *testing.T) {
	var errBuf bytes.Buffer
	code := fail(&errBuf, errors.New("boom"))
	if code != 1 {
		t.Errorf("fail() code = %d, want 1", code)
	}
	if !strings.Contains(errBuf.String(), "boom") {
		t.Errorf("fail() stderr = %q, want to contain 'boom'", errBuf.String())
	}
}

func TestRun_PDFFlagNoBlocks(t *testing.T) {
	// The -pdf flag with no mermaid blocks exits 1 via the runPDF path.
	dir := t.TempDir()
	mdPath := filepath.Join(dir, "plain.md")
	if err := os.WriteFile(mdPath, []byte("# just text\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errBuf bytes.Buffer
	code := run([]string{"-pdf", mdPath}, &out, &errBuf)
	if code != 1 {
		t.Errorf("code = %d, want 1", code)
	}
	if !strings.Contains(errBuf.String(), "no mermaid blocks") {
		t.Errorf("stderr = %q", errBuf.String())
	}
}

func TestRunPDF_NoMermaidBlocks(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := runPDF("# Just text, no diagrams\n", "", &out, &errBuf)
	if code != 1 {
		t.Errorf("code = %d, want 1 when no mermaid blocks", code)
	}
	if !strings.Contains(errBuf.String(), "no mermaid blocks") {
		t.Errorf("stderr = %q, want 'no mermaid blocks'", errBuf.String())
	}
}

func TestReplaceMarkersForPagerMode(t *testing.T) {
	markers := []string{"GLOWM_MERMAID_0"}
	images := [][]byte{[]byte("png")}
	output := "GLOWM_MERMAID_0"

	more := replaceMarkersForPagerMode(output, markers, images, nil, termimage.FormatKitty, 80, pager.ModeMore)
	if !strings.Contains(more, "glowm-rows=1") {
		t.Fatalf("more mode should include pager row metadata: %q", more)
	}

	vim := replaceMarkersForPagerMode(output, markers, images, nil, termimage.FormatKitty, 80, pager.ModeVim)
	if strings.Contains(vim, "glowm-rows=") {
		t.Fatalf("vim mode should not include pager row metadata: %q", vim)
	}
}

func TestFallbackText(t *testing.T) {
	cases := []struct {
		name string
		item markdown.MediaItem
		want string
	}{
		{
			name: "mermaid uses the shared placeholder",
			item: markdown.MediaItem{Kind: markdown.KindMermaid},
			want: markdown.Placeholder,
		},
		{
			name: "image with alt text names both",
			item: markdown.MediaItem{Kind: markdown.KindImage, Alt: "a cat", Source: "cat.png"},
			want: "[image: a cat (cat.png)]",
		},
		{
			name: "image without alt text names the source",
			item: markdown.MediaItem{Kind: markdown.KindImage, Source: "cat.png"},
			want: "[image: cat.png]",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := fallbackText(tc.item); got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestReplaceStrandedMarkers_SubstitutesFailedImage(t *testing.T) {
	items := []markdown.MediaItem{
		{Kind: markdown.KindImage, Marker: "GLOWM_IMAGE_0", Alt: "ok", Source: "ok.png"},
		{Kind: markdown.KindImage, Marker: "GLOWM_IMAGE_1", Alt: "gone", Source: "gone.png"},
	}
	images := [][]byte{[]byte("png"), nil}
	output := "  GLOWM_IMAGE_0  \n  GLOWM_IMAGE_1  "

	got := replaceStrandedMarkers(output, items, images)
	// The rendered marker stays for the image substitution that follows.
	if !strings.Contains(got, "GLOWM_IMAGE_0") {
		t.Fatal("expected the rendered marker to be left in place")
	}
	if strings.Contains(got, "GLOWM_IMAGE_1") {
		t.Fatal("expected the failed marker to be replaced")
	}
	if !strings.Contains(got, "[image: gone (gone.png)]") {
		t.Fatalf("expected fallback text, got %q", got)
	}
}

// The renderer splits a marker into separately styled runs at its underscores,
// so matching has to happen on ANSI-stripped text.
func TestReplaceStrandedMarkers_MatchesANSISplitMarker(t *testing.T) {
	items := []markdown.MediaItem{
		{Kind: markdown.KindImage, Marker: "GLOWM_IMAGE_0", Source: "gone.png"},
	}
	output := "  \x1b[38;5;252mGLOWM_\x1b[m\x1b[38;5;252mIMAGE_\x1b[m\x1b[38;5;252m0\x1b[m  "

	got := replaceStrandedMarkers(output, items, [][]byte{nil})
	if strings.Contains(termimage.StripANSI(got), "GLOWM_IMAGE_0") {
		t.Fatalf("expected the split marker to be replaced, got %q", got)
	}
	if !strings.Contains(got, "[image: gone.png]") {
		t.Fatalf("expected fallback text, got %q", got)
	}
}

// A marker that did not survive on a line of its own can never become an
// image, so it must be replaced with text even though it rendered fine.
func TestReplaceStrandedMarkers_SubstitutesMarkerSharingALine(t *testing.T) {
	items := []markdown.MediaItem{
		{Kind: markdown.KindImage, Marker: "GLOWM_IMAGE_0", Source: "a.png"},
	}
	output := "text GLOWM_IMAGE_0 more text"

	got := replaceStrandedMarkers(output, items, [][]byte{[]byte("png")})
	if strings.Contains(got, "GLOWM_IMAGE_0") {
		t.Fatalf("expected the stranded marker to be replaced, got %q", got)
	}
	if !strings.Contains(got, "text [image: a.png] more text") {
		t.Fatalf("expected surrounding text preserved, got %q", got)
	}
}

// Replacing "GLOWM_IMAGE_1" must not corrupt "GLOWM_IMAGE_10" beside it.
func TestReplaceStrandedMarkers_LongerIndexFirst(t *testing.T) {
	items := make([]markdown.MediaItem, 11)
	for i := range items {
		items[i] = markdown.MediaItem{
			Kind:   markdown.KindImage,
			Marker: fmt.Sprintf("GLOWM_IMAGE_%d", i),
			Source: fmt.Sprintf("%d.png", i),
		}
	}
	output := "x GLOWM_IMAGE_1 and GLOWM_IMAGE_10 y"

	got := replaceStrandedMarkers(output, items, make([][]byte, 11))
	if !strings.Contains(got, "[image: 1.png]") {
		t.Fatalf("expected 1.png fallback, got %q", got)
	}
	if !strings.Contains(got, "[image: 10.png]") {
		t.Fatalf("expected 10.png fallback, got %q", got)
	}
	if strings.Contains(got, "GLOWM_IMAGE") {
		t.Fatalf("expected no markers left, got %q", got)
	}
}

func TestReplaceStrandedMarkers_NoItems(t *testing.T) {
	if got := replaceStrandedMarkers("hello", nil, nil); got != "hello" {
		t.Fatalf("expected unchanged output, got %q", got)
	}
}

func TestCountRendered(t *testing.T) {
	if got := countRendered([][]byte{[]byte("a"), nil, {}, []byte("b")}); got != 2 {
		t.Fatalf("expected 2, got %d", got)
	}
}

func TestImageBounds(t *testing.T) {
	// Narrow terminals still get a usable minimum, matching the mermaid viewport.
	if w, h := imageBounds(10); w != 800 || h != 3200 {
		t.Fatalf("expected 800x3200 floor, got %dx%d", w, h)
	}
	if w, h := imageBounds(200); w != 1800 || h != 7200 {
		t.Fatalf("expected 1800x7200, got %dx%d", w, h)
	}
	if w, _ := imageBounds(0); w != 800 {
		t.Fatalf("expected 800 for unknown width, got %d", w)
	}
}

// Images are only inlined on a TTY with image support; the plain path must keep
// rendering the reference as text.
func TestRun_ImageReferenceOnNonTTY(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	if err := os.WriteFile(path, []byte("# T\n\n![a cat](cat.png)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := run([]string{path}, &stdout, &stderr); code != 0 {
		t.Fatalf("expected exit 0, got %d (stderr: %s)", code, stderr.String())
	}
	out := stdout.String()
	if strings.Contains(out, "GLOWM_") {
		t.Fatalf("expected no marker in output, got %q", out)
	}
	if !strings.Contains(out, "cat.png") {
		t.Fatalf("expected the image reference rendered as text, got %q", out)
	}
}
