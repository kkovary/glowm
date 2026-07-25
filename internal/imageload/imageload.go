// Package imageload resolves markdown image references to PNG bytes suitable
// for the terminal image encoders in internal/termimage.
package imageload

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	// Decoders for the formats commonly linked from markdown. Registering them
	// here keeps image.Decode able to sniff each one.
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
)

// maxFileSize caps how large a referenced image file may be before it is
// rejected unread, keeping a stray multi-hundred-megabyte file from being
// decoded into memory.
const maxFileSize = 32 * 1024 * 1024

// ErrRemote reports an image reference that points at a remote resource.
// Fetching over the network is not supported.
var ErrRemote = errors.New("remote image references are not supported")

// Load resolves ref, decodes it, and returns PNG bytes.
//
// A relative ref resolves against baseDir. maxWidth and maxHeight bound the
// pixel dimensions of the result; anything larger is downscaled, since the
// encoders base64 the full payload and terminals scale the image down to the
// display width anyway. Zero or negative bounds disable downscaling.
//
// An image that is already PNG and within bounds is returned byte-for-byte, so
// the common case costs one file read.
func Load(ref, baseDir string, maxWidth, maxHeight int) ([]byte, error) {
	path, err := resolve(ref, baseDir)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return nil, fmt.Errorf("%s is a directory", path)
	}
	if info.Size() > maxFileSize {
		return nil, fmt.Errorf("image too large: %d bytes (max %d)", info.Size(), maxFileSize)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	cfg, format, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("decoding %s: %w", filepath.Base(path), err)
	}
	if format == "png" && withinBounds(cfg.Width, cfg.Height, maxWidth, maxHeight) {
		return raw, nil
	}

	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("decoding %s: %w", filepath.Base(path), err)
	}
	img = fit(img, maxWidth, maxHeight)

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("encoding %s as png: %w", filepath.Base(path), err)
	}
	return buf.Bytes(), nil
}

// resolve turns a markdown image reference into a filesystem path.
func resolve(ref, baseDir string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", errors.New("empty image reference")
	}
	if isRemote(ref) {
		return "", ErrRemote
	}
	// Strip a fragment, which carries no meaning for a local file. Query
	// strings are left alone: '?' is legal in a filename and markdown
	// referencing a local image rarely appends one.
	if i := strings.IndexByte(ref, '#'); i >= 0 {
		ref = ref[:i]
	}
	// Markdown sources are URL-ish, so a path with spaces may arrive
	// percent-encoded. Fall back to the raw text when it is not valid
	// encoding, which is the more likely reading of a literal '%'.
	if decoded, err := url.PathUnescape(ref); err == nil {
		ref = decoded
	}
	if ref == "" {
		return "", errors.New("empty image reference")
	}
	if strings.HasPrefix(ref, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			ref = filepath.Join(home, ref[2:])
		}
	}
	if filepath.IsAbs(ref) {
		return filepath.Clean(ref), nil
	}
	if baseDir == "" {
		baseDir = "."
	}
	return filepath.Join(baseDir, ref), nil
}

// isRemote reports whether ref names a scheme this package cannot read from
// disk. A bare Windows-style drive letter is not treated as a scheme.
func isRemote(ref string) bool {
	lower := strings.ToLower(ref)
	for _, scheme := range []string{"http://", "https://", "data:", "ftp://"} {
		if strings.HasPrefix(lower, scheme) {
			return true
		}
	}
	return false
}

func withinBounds(w, h, maxWidth, maxHeight int) bool {
	if maxWidth > 0 && w > maxWidth {
		return false
	}
	if maxHeight > 0 && h > maxHeight {
		return false
	}
	return true
}

// fit downscales img to sit within maxWidth by maxHeight, preserving aspect
// ratio. Images already within bounds are returned unchanged; fit never
// upscales.
func fit(img image.Image, maxWidth, maxHeight int) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 || withinBounds(w, h, maxWidth, maxHeight) {
		return img
	}

	scale := 1.0
	if maxWidth > 0 && w > maxWidth {
		scale = float64(maxWidth) / float64(w)
	}
	if maxHeight > 0 && h > maxHeight {
		if s := float64(maxHeight) / float64(h); s < scale {
			scale = s
		}
	}

	dstW := int(float64(w) * scale)
	dstH := int(float64(h) * scale)
	if dstW < 1 {
		dstW = 1
	}
	if dstH < 1 {
		dstH = 1
	}
	return boxDownscale(img, dstW, dstH)
}

// boxDownscale resizes img to dstW by dstH by averaging each destination
// pixel's source rectangle. Averaging rather than point sampling matters for
// screenshots of text, where nearest-neighbour aliases badly.
func boxDownscale(img image.Image, dstW, dstH int) image.Image {
	b := img.Bounds()
	dst := image.NewNRGBA(image.Rect(0, 0, dstW, dstH))

	for dy := 0; dy < dstH; dy++ {
		sy0 := b.Min.Y + dy*b.Dy()/dstH
		sy1 := b.Min.Y + (dy+1)*b.Dy()/dstH
		if sy1 <= sy0 {
			sy1 = sy0 + 1
		}
		for dx := 0; dx < dstW; dx++ {
			sx0 := b.Min.X + dx*b.Dx()/dstW
			sx1 := b.Min.X + (dx+1)*b.Dx()/dstW
			if sx1 <= sx0 {
				sx1 = sx0 + 1
			}

			var r, g, bl, a uint64
			var n uint64
			for sy := sy0; sy < sy1; sy++ {
				for sx := sx0; sx < sx1; sx++ {
					// RGBA returns alpha-premultiplied 16-bit values, which is
					// what we want to average: averaging un-premultiplied
					// colour bleeds transparent pixels into the result.
					pr, pg, pb, pa := img.At(sx, sy).RGBA()
					r += uint64(pr)
					g += uint64(pg)
					bl += uint64(pb)
					a += uint64(pa)
					n++
				}
			}
			if n == 0 {
				continue
			}
			dst.Set(dx, dy, premulToNRGBA(r/n, g/n, bl/n, a/n))
		}
	}
	return dst
}

// premulToNRGBA converts averaged 16-bit alpha-premultiplied components back
// to the un-premultiplied 8-bit form image.NRGBA stores.
func premulToNRGBA(r, g, b, a uint64) color.NRGBA {
	if a == 0 {
		return color.NRGBA{}
	}
	return color.NRGBA{
		R: unpremul(r, a),
		G: unpremul(g, a),
		B: unpremul(b, a),
		A: uint8(a >> 8),
	}
}

func unpremul(c, a uint64) uint8 {
	v := c * 0xffff / a >> 8
	if v > 0xff {
		return 0xff
	}
	return uint8(v)
}
