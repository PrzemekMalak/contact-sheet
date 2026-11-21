// Package xexif extracts display metadata from JPEG EXIF.
package xexif

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/rwcarlsen/goexif/exif"
)

// decode reads EXIF from path. A file without EXIF yields (nil, nil).
func decode(path string) (*exif.Exif, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	x, err := exif.Decode(f)
	if err != nil {
		// A JPEG with no EXIF yields EOF or an "intro marker" error. A
		// malformed/truncated APP1 is deliberately treated as "no EXIF" too,
		// rather than failing the caller.
		if errors.Is(err, io.EOF) || strings.Contains(err.Error(), "exif intro marker") {
			return nil, nil
		}
		// Non-critical errors (e.g. unknown tags) still yield usable data.
		if x == nil || exif.IsCriticalError(err) {
			return nil, fmt.Errorf("read exif %s: %w", path, err)
		}
	}
	return x, nil
}

func str(x *exif.Exif, name exif.FieldName) string {
	t, err := x.Get(name)
	if err != nil {
		return ""
	}
	s, err := t.StringVal()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(s)
}

// rat returns the first rational of a tag as numerator/denominator.
func rat(x *exif.Exif, name exif.FieldName) (n, d int64, ok bool) {
	t, err := x.Get(name)
	if err != nil {
		return 0, 0, false
	}
	n, d, err = t.Rat2(0)
	if err != nil || d == 0 {
		return 0, 0, false
	}
	return n, d, true
}

// formatNum renders n/d as a decimal with trailing zeros trimmed (6.3, 4).
func formatNum(n, d int64) string {
	s := strconv.FormatFloat(float64(n)/float64(d), 'f', 2, 64)
	return strings.TrimSuffix(strings.TrimRight(s, "0"), ".")
}

// Extract returns camera, lens, aperture, shutter, iso and focal keys,
// omitting any whose tag is absent.
func Extract(path string) (map[string]any, error) {
	out := map[string]any{}
	x, err := decode(path)
	if err != nil || x == nil {
		return out, err
	}

	if cam := strings.TrimSpace(str(x, exif.Make) + " " + str(x, exif.Model)); cam != "" {
		out["camera"] = cam
	}
	if lens := str(x, exif.LensModel); lens != "" {
		out["lens"] = lens
	}
	if n, d, ok := rat(x, exif.FNumber); ok {
		out["aperture"] = "f/" + formatNum(n, d)
	}
	if n, d, ok := rat(x, exif.ExposureTime); ok && n > 0 {
		if n < d {
			out["shutter"] = fmt.Sprintf("1/%s", formatNum(d, n))
		} else {
			out["shutter"] = formatNum(n, d)
		}
	}
	if t, err := x.Get(exif.ISOSpeedRatings); err == nil {
		if iso, err := t.Int(0); err == nil && iso > 0 {
			out["iso"] = iso
		}
	}
	if n, d, ok := rat(x, exif.FocalLength); ok {
		out["focal"] = formatNum(n, d) + "mm"
	}
	return out, nil
}

// Taken returns the capture time as YYYY-MM-DDTHH:MM:SS from DateTimeOriginal,
// falling back to DateTime. It returns "" (no error) when neither is present.
func Taken(path string) (string, error) {
	x, err := decode(path)
	if err != nil || x == nil {
		return "", err
	}
	for _, name := range []exif.FieldName{exif.DateTimeOriginal, exif.DateTime} {
		s := str(x, name)
		if s == "" {
			continue
		}
		t, err := time.Parse("2006:01:02 15:04:05", s)
		if err != nil {
			return "", fmt.Errorf("parse %s %q: %w", name, s, err)
		}
		return t.Format("2006-01-02T15:04:05"), nil
	}
	return "", nil
}
