package xexif

import (
	"reflect"
	"testing"
)

func TestExtractFields(t *testing.T) {
	got, err := Extract("testdata/full.jpg")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"camera":   "SONY ILCE-6700",
		"lens":     "FE 400mm F4.5 GM OSS + 1.4X Teleconverter",
		"aperture": "f/6.3",
		"shutter":  "1/320",
		"iso":      100,
		"focal":    "560mm",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v\nwant %#v", got, want)
	}
}

func TestExtractOmitsMissing(t *testing.T) {
	got, err := Extract("testdata/nodate.jpg")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"camera": "SONY ILCE-6700"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestTakenFromOriginal(t *testing.T) {
	got, err := Taken("testdata/full.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if got != "2026-10-21T11:28:21" {
		t.Fatalf("got %q", got)
	}
}

func TestTakenMissingReturnsEmpty(t *testing.T) {
	got, err := Taken("testdata/nodate.jpg")
	if err != nil || got != "" {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestNoExif(t *testing.T) {
	got, err := Extract("testdata/noexif.jpg")
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("Extract = %#v, %v", got, err)
	}
	taken, err := Taken("testdata/noexif.jpg")
	if err != nil || taken != "" {
		t.Fatalf("Taken = %q, %v", taken, err)
	}
}

func TestTakenFallsBackToDateTime(t *testing.T) {
	got, err := Taken("testdata/dtonly.jpg")
	if err != nil || got != "2025-01-02T03:04:05" {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestFormatNum(t *testing.T) {
	for _, c := range []struct {
		n, d int64
		want string
	}{{63, 10, "6.3"}, {4, 1, "4"}, {13, 10, "1.3"}, {2, 1, "2"}, {560, 1, "560"}} {
		if got := formatNum(c.n, c.d); got != c.want {
			t.Errorf("%d/%d = %q want %q", c.n, c.d, got, c.want)
		}
	}
}
