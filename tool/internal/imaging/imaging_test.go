package imaging

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gen2brain/webp"
)

func decodeWebP(t *testing.T, p string) (int, int) {
	t.Helper()
	f, err := os.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := webp.Decode(f)
	if err != nil {
		t.Fatalf("decode webp: %v", err)
	}
	b := img.Bounds()
	return b.Dx(), b.Dy()
}

func TestDimensions(t *testing.T) {
	w, h, err := Dimensions("testdata/landscape.jpg")
	if err != nil || w != 300 || h != 200 {
		t.Fatalf("got %d,%d,%v want 300,200", w, h, err)
	}
}

func TestMakeDerivativeResizesAndEncodes(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "out.webp")
	w, h, err := MakeDerivative("testdata/landscape.jpg", dest, 150, 82)
	if err != nil {
		t.Fatal(err)
	}
	if w != 150 || h != 100 {
		t.Fatalf("returned %d,%d want 150,100", w, h)
	}
	dw, dh := decodeWebP(t, dest)
	if dw != w || dh != h {
		t.Fatalf("decoded %d,%d != returned %d,%d", dw, dh, w, h)
	}
}

func TestMakeDerivativeNoUpscale(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "out.webp")
	w, h, err := MakeDerivative("testdata/landscape.jpg", dest, 1600, 82)
	if err != nil || w != 300 || h != 200 {
		t.Fatalf("got %d,%d,%v want 300,200", w, h, err)
	}
}

func TestMakeDerivativeOrientsPortrait(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "out.webp")
	w, h, err := MakeDerivative("testdata/orient6.jpg", dest, 1600, 82)
	if err != nil {
		t.Fatal(err)
	}
	if !(w < h) || w != 100 || h != 200 {
		t.Fatalf("got %d,%d want upright 100,200", w, h)
	}
	dw, dh := decodeWebP(t, dest)
	if dw != w || dh != h {
		t.Fatalf("decoded %d,%d != returned %d,%d", dw, dh, w, h)
	}
}
