package imageload

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// writePNG writes a w by h PNG filled with a solid colour and returns its path.
func writePNG(t *testing.T, dir, name string, w, h int, c color.Color) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func decodeSize(t *testing.T, b []byte) (int, int) {
	t.Helper()
	cfg, _, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("decoding result: %v", err)
	}
	return cfg.Width, cfg.Height
}

func TestLoadPNGPassthrough(t *testing.T) {
	dir := t.TempDir()
	path := writePNG(t, dir, "a.png", 40, 20, color.RGBA{R: 255, A: 255})
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	got, err := Load("a.png", dir, 800, 3200)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Equal(got, original) {
		t.Fatal("expected an in-bounds PNG to be returned byte-for-byte")
	}
}

func TestLoadConvertsJPEGToPNG(t *testing.T) {
	dir := t.TempDir()
	img := image.NewRGBA(image.Rect(0, 0, 30, 15))
	for y := 0; y < 15; y++ {
		for x := 0; x < 30; x++ {
			img.Set(x, y, color.RGBA{G: 200, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.jpg"), buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := Load("a.jpg", dir, 800, 3200)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The terminal encoders assume PNG, so a JPEG source must be re-encoded.
	if _, format, err := image.DecodeConfig(bytes.NewReader(got)); err != nil || format != "png" {
		t.Fatalf("expected png, got format %q err %v", format, err)
	}
	if w, h := decodeSize(t, got); w != 30 || h != 15 {
		t.Fatalf("expected 30x15, got %dx%d", w, h)
	}
}

func TestLoadConvertsGIFToPNG(t *testing.T) {
	dir := t.TempDir()
	img := image.NewPaletted(image.Rect(0, 0, 12, 8), color.Palette{color.Black, color.White})
	var buf bytes.Buffer
	if err := gif.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.gif"), buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := Load("a.gif", dir, 800, 3200)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, format, err := image.DecodeConfig(bytes.NewReader(got)); err != nil || format != "png" {
		t.Fatalf("expected png, got format %q err %v", format, err)
	}
}

// Fixtures here stay small on purpose. Every pixel costs two instrumented
// accesses under -race (one to build the fixture, one to average it), so a
// megapixel fixture turns a millisecond of scaling math into ten seconds of CPU
// and starves the Chrome-backed tests running in parallel packages.
func TestLoadDownscalesWideImage(t *testing.T) {
	dir := t.TempDir()
	writePNG(t, dir, "wide.png", 400, 300, color.RGBA{B: 255, A: 255})

	got, err := Load("wide.png", dir, 100, 400)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	w, h := decodeSize(t, got)
	if w != 100 {
		t.Fatalf("expected width 100, got %d", w)
	}
	// Aspect ratio preserved: 300/400 * 100 = 75.
	if h != 75 {
		t.Fatalf("expected height 75, got %d", h)
	}
}

func TestLoadDownscalesTallImage(t *testing.T) {
	dir := t.TempDir()
	writePNG(t, dir, "tall.png", 100, 500, color.RGBA{A: 255})

	got, err := Load("tall.png", dir, 900, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	w, h := decodeSize(t, got)
	if h != 100 {
		t.Fatalf("expected height 100, got %d", h)
	}
	// Height is the binding bound: 100/500 scales the width to 20.
	if w != 20 {
		t.Fatalf("expected width 20, got %d", w)
	}
}

func TestLoadNeverUpscales(t *testing.T) {
	dir := t.TempDir()
	writePNG(t, dir, "small.png", 10, 10, color.RGBA{A: 255})

	got, err := Load("small.png", dir, 900, 3600)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if w, h := decodeSize(t, got); w != 10 || h != 10 {
		t.Fatalf("expected 10x10, got %dx%d", w, h)
	}
}

func TestLoadZeroBoundsDisablesDownscaling(t *testing.T) {
	dir := t.TempDir()
	writePNG(t, dir, "wide.png", 300, 50, color.RGBA{A: 255})

	got, err := Load("wide.png", dir, 0, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if w, _ := decodeSize(t, got); w != 300 {
		t.Fatalf("expected width 300, got %d", w)
	}
}

// Downscaling averages the source rectangle, so a solid colour must survive it
// unchanged rather than drifting.
func TestDownscalePreservesSolidColor(t *testing.T) {
	dir := t.TempDir()
	want := color.RGBA{R: 10, G: 120, B: 240, A: 255}
	writePNG(t, dir, "solid.png", 120, 120, want)

	got, err := Load("solid.png", dir, 30, 30)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	img, err := png.Decode(bytes.NewReader(got))
	if err != nil {
		t.Fatal(err)
	}
	r, g, b, a := img.At(15, 15).RGBA()
	if uint8(r>>8) != want.R || uint8(g>>8) != want.G || uint8(b>>8) != want.B || uint8(a>>8) != want.A {
		t.Fatalf("expected %v, got rgba(%d,%d,%d,%d)", want, r>>8, g>>8, b>>8, a>>8)
	}
}

func TestLoadResolvesRelativeToBaseDir(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "img")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	writePNG(t, sub, "a.png", 5, 5, color.RGBA{A: 255})

	if _, err := Load("img/a.png", dir, 800, 3200); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The same reference must not resolve from an unrelated base directory.
	if _, err := Load("img/a.png", t.TempDir(), 800, 3200); err == nil {
		t.Fatal("expected error resolving from a different base directory")
	}
}

func TestLoadAbsolutePathIgnoresBaseDir(t *testing.T) {
	dir := t.TempDir()
	path := writePNG(t, dir, "a.png", 5, 5, color.RGBA{A: 255})

	if _, err := Load(path, t.TempDir(), 800, 3200); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadPercentEncodedPath(t *testing.T) {
	dir := t.TempDir()
	writePNG(t, dir, "my image.png", 5, 5, color.RGBA{A: 255})

	if _, err := Load("my%20image.png", dir, 800, 3200); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// A literal '%' is more likely than broken encoding, so an un-decodable
// reference falls back to the raw text.
func TestLoadLiteralPercentInFilename(t *testing.T) {
	dir := t.TempDir()
	writePNG(t, dir, "100%.png", 5, 5, color.RGBA{A: 255})

	if _, err := Load("100%.png", dir, 800, 3200); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadStripsFragment(t *testing.T) {
	dir := t.TempDir()
	writePNG(t, dir, "a.png", 5, 5, color.RGBA{A: 255})

	if _, err := Load("a.png#anchor", dir, 800, 3200); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadRejectsRemoteReferences(t *testing.T) {
	for _, ref := range []string{
		"https://example.com/a.png",
		"http://example.com/a.png",
		"HTTPS://EXAMPLE.COM/a.png",
		"data:image/png;base64,iVBORw0KGgo=",
		"ftp://example.com/a.png",
	} {
		t.Run(ref, func(t *testing.T) {
			_, err := Load(ref, t.TempDir(), 800, 3200)
			if !errors.Is(err, ErrRemote) {
				t.Fatalf("expected ErrRemote, got %v", err)
			}
		})
	}
}

func TestLoadErrors(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "adir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notimage.png"), []byte("not an image"), 0o644); err != nil {
		t.Fatal(err)
	}

	cases := []struct{ name, ref string }{
		{"missing file", "nope.png"},
		{"empty reference", ""},
		{"whitespace reference", "   "},
		{"directory", "adir"},
		{"undecodable content", "notimage.png"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Load(tc.ref, dir, 800, 3200); err == nil {
				t.Fatalf("expected an error for %q", tc.ref)
			}
		})
	}
}
