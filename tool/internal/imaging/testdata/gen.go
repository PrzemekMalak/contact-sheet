//go:build ignore

// gen writes the JPEG fixtures used by imaging tests.
// Run from tool/internal/imaging: go run testdata/gen.go
package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"os"
)

func gradient(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{uint8(x * 255 / w), uint8(y * 255 / h), 128, 255})
		}
	}
	return img
}

func encode(img image.Image) []byte {
	var b bytes.Buffer
	must(jpeg.Encode(&b, img, &jpeg.Options{Quality: 85}))
	return b.Bytes()
}

// withOrientation splices a minimal big-endian EXIF APP1 (Orientation only) after SOI.
func withOrientation(jpg []byte, orient uint16) []byte {
	tiff := []byte{'M', 'M', 0, 42, 0, 0, 0, 8}
	tiff = binary.BigEndian.AppendUint16(tiff, 1)      // one entry
	tiff = binary.BigEndian.AppendUint16(tiff, 0x0112) // Orientation
	tiff = binary.BigEndian.AppendUint16(tiff, 3)      // SHORT
	tiff = binary.BigEndian.AppendUint32(tiff, 1)      // count
	tiff = binary.BigEndian.AppendUint16(tiff, orient) // value
	tiff = append(tiff, 0, 0)                          // value pad
	tiff = append(tiff, 0, 0, 0, 0)                    // next IFD
	app1 := append([]byte("Exif\x00\x00"), tiff...)
	out := []byte{0xFF, 0xD8, 0xFF, 0xE1}
	out = binary.BigEndian.AppendUint16(out, uint16(len(app1)+2))
	out = append(out, app1...)
	return append(out, jpg[2:]...)
}

func main() {
	must(os.WriteFile("testdata/landscape.jpg", encode(gradient(300, 200)), 0o644))
	// Stored 200x100 landscape, Orientation 6 => upright 100x200 portrait.
	must(os.WriteFile("testdata/orient6.jpg", withOrientation(encode(gradient(200, 100)), 6), 0o644))
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
