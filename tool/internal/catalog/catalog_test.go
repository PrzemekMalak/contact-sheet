package catalog

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/PrzemekMalak/contact-sheet/tool/internal/config"
)

func tempCfg(t *testing.T) *config.Config {
	t.Helper()
	return &config.Config{PhotosIndex: filepath.Join(t.TempDir(), "photos.json")}
}

func TestSaveSortsByID(t *testing.T) {
	cfg := tempCfg(t)
	in := []Record{{ID: "c"}, {ID: "a"}, {ID: "b"}}
	if err := Save(cfg, in); err != nil {
		t.Fatal(err)
	}
	out, err := Load(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, r := range out {
		ids = append(ids, r.ID)
	}
	if !reflect.DeepEqual(ids, []string{"a", "b", "c"}) {
		t.Fatalf("got %v", ids)
	}
}

func TestUpsertReplacesByID(t *testing.T) {
	ps := []Record{{ID: "a", Caption: "old"}, {ID: "b"}}
	ps = Upsert(ps, Record{ID: "a", Caption: "new"})
	if len(ps) != 2 || ps[0].Caption != "new" {
		t.Fatalf("got %+v", ps)
	}
	ps = Upsert(ps, Record{ID: "z"})
	if len(ps) != 3 || ps[2].ID != "z" {
		t.Fatalf("got %+v", ps)
	}
}

func TestRemoveDropsRecord(t *testing.T) {
	ps := Remove([]Record{{ID: "a"}, {ID: "b"}}, "a")
	if len(ps) != 1 || ps[0].ID != "b" {
		t.Fatalf("got %+v", ps)
	}
	if _, ok := Get(ps, "a"); ok {
		t.Fatal("a still present")
	}
}

func TestAddAlbumDedupes(t *testing.T) {
	r := Record{Albums: []string{"x"}}
	AddAlbum(&r, "x")
	if !reflect.DeepEqual(r.Albums, []string{"x"}) {
		t.Fatalf("got %v", r.Albums)
	}
	AddAlbum(&r, "y")
	if !reflect.DeepEqual(r.Albums, []string{"x", "y"}) {
		t.Fatalf("got %v", r.Albums)
	}
}

func TestRemoveAlbum(t *testing.T) {
	r := Record{Albums: []string{"x", "y", "z"}}
	RemoveAlbum(&r, "y")
	if !reflect.DeepEqual(r.Albums, []string{"x", "z"}) {
		t.Fatalf("got %v", r.Albums)
	}
}

func TestSaveDoesNotEscapeHTML(t *testing.T) {
	cfg := tempCfg(t)
	if err := Save(cfg, []Record{{ID: "a", Caption: "a & b <c>"}}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(cfg.PhotosIndex)
	if !strings.Contains(string(b), "a & b <c>") {
		t.Fatalf("caption escaped: %s", b)
	}
	if strings.Contains(string(b), `\u00`) {
		t.Fatalf("unexpected unicode escape: %s", b)
	}
}

func TestTakenOmittedWhenEmpty(t *testing.T) {
	cfg := tempCfg(t)
	if err := Save(cfg, []Record{{ID: "a", Tags: []string{}, Albums: []string{}}}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(cfg.PhotosIndex)
	if strings.Contains(string(b), `"taken"`) {
		t.Fatalf("taken present: %s", b)
	}
	if err := Save(cfg, []Record{{ID: "a", Taken: "2026-01-01T00:00:00"}}); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(cfg.PhotosIndex)
	if !strings.Contains(string(b), `"taken": "2026-01-01T00:00:00"`) {
		t.Fatalf("taken missing: %s", b)
	}
}

func TestSaveExifKeyOrder(t *testing.T) {
	cfg := tempCfg(t)
	// Keys supplied in a scrambled order; Save must emit them in the fixed
	// order: camera, lens, aperture, shutter, iso, focal.
	rec := Record{ID: "p1", Tags: []string{}, Albums: []string{}, Exif: Exif{
		"iso": 100, "focal": "560mm", "camera": "SONY ILCE-6700",
		"shutter": "1/320", "lens": "FE 400mm", "aperture": "f/6.3",
	}}
	if err := Save(cfg, []Record{rec}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(cfg.PhotosIndex)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	last := -1
	for _, k := range []string{"camera", "lens", "aperture", "shutter", "iso", "focal"} {
		i := strings.Index(s, `"`+k+`"`)
		if i < 0 {
			t.Fatalf("exif key %q missing from output", k)
		}
		if i < last {
			t.Fatalf("exif key %q is out of order (want camera,lens,aperture,shutter,iso,focal)", k)
		}
		last = i
	}
}
