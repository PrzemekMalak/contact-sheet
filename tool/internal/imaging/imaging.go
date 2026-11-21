// Package imaging produces WebP derivatives from source JPEGs.
package imaging

import (
	"image"
	"image/jpeg"
	"os"

	"github.com/gen2brain/webp"
	"github.com/rwcarlsen/goexif/exif"
	"golang.org/x/image/draw"
)

// Dimensions returns the stored pixel dimensions of the image at path.
func Dimensions(path string) (w, h int, err error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return 0, 0, err
	}
	return cfg.Width, cfg.Height, nil
}

// MakeDerivative decodes the JPEG at srcPath, applies its EXIF orientation,
// downsizes so the longest edge is <= maxEdge (never upscales), writes a lossy
// WebP to destPath, and returns the written dimensions.
func MakeDerivative(srcPath, destPath string, maxEdge, quality int) (w, h int, err error) {
	f, err := os.Open(srcPath)
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()
	src, err := jpeg.Decode(f)
	if err != nil {
		return 0, 0, err
	}
	img := orient(src, readOrientation(srcPath))

	b := img.Bounds()
	w, h = b.Dx(), b.Dy()
	if long := max(w, h); long > maxEdge {
		w = max(1, (w*maxEdge+long/2)/long)
		h = max(1, (h*maxEdge+long/2)/long)
		dst := image.NewRGBA(image.Rect(0, 0, w, h))
		draw.CatmullRom.Scale(dst, dst.Bounds(), img, b, draw.Src, nil)
		img = dst
	}

	out, err := os.Create(destPath)
	if err != nil {
		return 0, 0, err
	}
	if err := webp.Encode(out, img, webp.Options{Quality: quality}); err != nil {
		out.Close()
		return 0, 0, err
	}
	if err := out.Close(); err != nil {
		return 0, 0, err
	}
	return w, h, nil
}

// readOrientation returns the EXIF Orientation (1-8); missing EXIF or tag => 1.
func readOrientation(path string) int {
	f, err := os.Open(path)
	if err != nil {
		return 1
	}
	defer f.Close()
	x, err := exif.Decode(f)
	if err != nil {
		return 1
	}
	tag, err := x.Get(exif.Orientation)
	if err != nil {
		return 1
	}
	o, err := tag.Int(0)
	if err != nil || o < 1 || o > 8 {
		return 1
	}
	return o
}

// orient returns src transformed to upright per EXIF orientation o (1-8).
func orient(src image.Image, o int) image.Image {
	if o == 1 {
		return src
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	dw, dh := w, h
	if o >= 5 {
		dw, dh = h, w
	}
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := 0; y < dh; y++ {
		for x := 0; x < dw; x++ {
			var sx, sy int
			switch o {
			case 2: // mirror horizontal
				sx, sy = w-1-x, y
			case 3: // rotate 180
				sx, sy = w-1-x, h-1-y
			case 4: // mirror vertical
				sx, sy = x, h-1-y
			case 5: // transpose
				sx, sy = y, x
			case 6: // rotate 90 CW
				sx, sy = y, h-1-x
			case 7: // transverse
				sx, sy = h-1-y, w-1-x
			case 8: // rotate 90 CCW
				sx, sy = w-1-y, x
			}
			dst.Set(x, y, src.At(b.Min.X+sx, b.Min.Y+sy))
		}
	}
	return dst
}
