// Package catalog loads and saves the photo index (data/photos.json) and
// provides membership helpers.
package catalog

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"

	"github.com/PrzemekMalak/contact-sheet/tool/internal/config"
)

// Record is one photo. Field order defines JSON key order on write.
type Record struct {
	ID       string   `json:"id"`
	Thumb    string   `json:"thumb"`
	Preview  string   `json:"preview"`
	Original string   `json:"original"`
	Width    int      `json:"width"`
	Height   int      `json:"height"`
	PW       int      `json:"pw"`
	PH       int      `json:"ph"`
	Caption  string   `json:"caption"`
	Tags     []string `json:"tags"`
	Albums   []string `json:"albums"`
	Featured bool     `json:"featured"`
	Exif     Exif     `json:"exif"`
	Taken    string   `json:"taken,omitempty"`
}

// Exif is a photo's EXIF fields. It marshals with keys in a fixed order
// (camera, lens, aperture, shutter, iso, focal) rather than
// Go's default alphabetical map order, so data/photos.json diffs stay clean.
// Any extra keys follow, sorted, so none are ever dropped.
type Exif map[string]any

var exifKeyOrder = []string{"camera", "lens", "aperture", "shutter", "iso", "focal"}

func (e Exif) MarshalJSON() ([]byte, error) {
	if e == nil {
		return []byte("null"), nil
	}
	ordered := make([]string, 0, len(e))
	seen := make(map[string]bool, len(e))
	for _, k := range exifKeyOrder {
		if _, ok := e[k]; ok {
			ordered = append(ordered, k)
			seen[k] = true
		}
	}
	var extra []string
	for k := range e {
		if !seen[k] {
			extra = append(extra, k)
		}
	}
	sort.Strings(extra)
	ordered = append(ordered, extra...)

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	buf.WriteByte('{')
	for i, k := range ordered {
		if i > 0 {
			buf.WriteByte(',')
		}
		kb, err := json.Marshal(k)
		if err != nil {
			return nil, err
		}
		buf.Write(kb)
		buf.WriteByte(':')
		// Encode writes a trailing newline; trim it.
		start := buf.Len()
		if err := enc.Encode(e[k]); err != nil {
			return nil, err
		}
		b := buf.Bytes()
		for len(b) > start && (b[len(b)-1] == '\n') {
			b = b[:len(b)-1]
		}
		buf.Truncate(len(b))
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// Load reads cfg.PhotosIndex (relative to the current working directory).
func Load(cfg *config.Config) ([]Record, error) {
	b, err := os.ReadFile(cfg.PhotosIndex)
	if err != nil {
		return nil, err
	}
	var photos []Record
	if err := json.Unmarshal(b, &photos); err != nil {
		return nil, fmt.Errorf("parse %s: %w", cfg.PhotosIndex, err)
	}
	return photos, nil
}

// Save writes photos sorted by ID, indented 2 spaces, with a trailing newline,
// without HTML escaping (& < > stay raw).
// The caller's slice is not reordered.
func Save(cfg *config.Config, photos []Record) error {
	sorted := slices.Clone(photos)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(sorted); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(cfg.PhotosIndex), 0o755); err != nil {
		return err
	}
	return os.WriteFile(cfg.PhotosIndex, buf.Bytes(), 0o644)
}

// Get returns the record with the given ID.
func Get(photos []Record, id string) (Record, bool) {
	for _, r := range photos {
		if r.ID == id {
			return r, true
		}
	}
	return Record{}, false
}

// Upsert replaces the record with rec.ID, or appends it if absent.
func Upsert(photos []Record, rec Record) []Record {
	for i := range photos {
		if photos[i].ID == rec.ID {
			photos[i] = rec
			return photos
		}
	}
	return append(photos, rec)
}

// Remove drops the record with the given ID.
func Remove(photos []Record, id string) []Record {
	return slices.DeleteFunc(photos, func(r Record) bool { return r.ID == id })
}

// AddAlbum appends slug to rec.Albums unless already present.
func AddAlbum(rec *Record, slug string) {
	if !slices.Contains(rec.Albums, slug) {
		rec.Albums = append(rec.Albums, slug)
	}
}

// RemoveAlbum removes slug from rec.Albums, preserving order.
func RemoveAlbum(rec *Record, slug string) {
	rec.Albums = slices.DeleteFunc(rec.Albums, func(s string) bool { return s == slug })
}
