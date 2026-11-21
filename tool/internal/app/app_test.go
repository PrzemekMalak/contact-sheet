package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PrzemekMalak/contact-sheet/tool/internal/catalog"
	"github.com/PrzemekMalak/contact-sheet/tool/internal/config"
	"github.com/PrzemekMalak/contact-sheet/tool/internal/store"
)

// setup creates a temp local-mode repo, chdirs into it, and returns the
// config plus a helper-copied source jpg path.
func setup(t *testing.T) (*config.Config, store.Store, string) {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	toml := "[project]\nassets_dir = \"assets/gallery\"\nphotos_index = \"data/photos.json\"\nalbums_dir = \"content/albums\"\ntags_dir = \"content/tags\"\n" +
		"[images]\nthumb_max_edge = 100\npreview_max_edge = 200\nwebp_quality = 80\n[s3]\nbucket = \"\"\n"
	must(t, os.WriteFile("contact-sheet.toml", []byte(toml), 0o644))
	for _, d := range []string{"assets/gallery", "content/albums", "content/tags", "data"} {
		must(t, os.MkdirAll(d, 0o755))
	}
	must(t, os.WriteFile("data/photos.json", []byte("[]\n"), 0o644))
	cfg, err := config.Load(".")
	must(t, err)
	st, err := store.New(cfg)
	must(t, err)
	return cfg, st, ""
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// fixture copies an imaging testdata jpg into the temp repo under name.
func fixture(t *testing.T, src, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(origWD, "..", "imaging", "testdata", src))
	must(t, err)
	must(t, os.WriteFile(name, b, 0o644))
	return name
}

var origWD string

func init() { origWD, _ = os.Getwd() }

func newRepo(t *testing.T) (*config.Config, store.Store) {
	cfg, st, _ := setup(t)
	return cfg, st
}

func TestAddCreatesRecordAndDerivatives(t *testing.T) {
	cfg, st := newRepo(t)
	p := fixture(t, "landscape.jpg", "p1.jpg")
	recs, err := Add(cfg, st, []string{p}, AddOpts{WithExif: true})
	must(t, err)
	if len(recs) != 1 {
		t.Fatalf("got %d records", len(recs))
	}
	r := recs[0]
	if r.ID != "p1" || r.Thumb != "gallery/p1-thumb.webp" || r.Preview != "gallery/p1-preview.webp" {
		t.Fatalf("bad record %+v", r)
	}
	for _, f := range []string{"assets/gallery/p1-thumb.webp", "assets/gallery/p1-preview.webp"} {
		if _, err := os.Stat(f); err != nil {
			t.Errorf("missing %s", f)
		}
	}
	if r.Tags == nil || r.Albums == nil || r.Exif == nil {
		t.Errorf("nil slice/map: %+v", r)
	}
	photos, err := catalog.Load(cfg)
	must(t, err)
	if len(photos) != 1 {
		t.Fatalf("catalog has %d", len(photos))
	}
	if raw, _ := os.ReadFile("data/photos.json"); !contains(raw, `"tags": []`) || !contains(raw, `"exif": {}`) {
		t.Errorf("expected [] and {} in json: %s", raw)
	}
}

func contains(b []byte, s string) bool { return strings.Contains(string(b), s) }

func TestAddMissingDateOmitsTaken(t *testing.T) {
	cfg, st := newRepo(t)
	p := fixture(t, "landscape.jpg", "nodate.jpg")
	recs, err := Add(cfg, st, []string{p}, AddOpts{WithExif: true})
	must(t, err)
	if recs[0].Taken != "" {
		t.Errorf("Taken = %q", recs[0].Taken)
	}
	if raw, _ := os.ReadFile("data/photos.json"); contains(raw, `"taken"`) {
		t.Errorf("taken should be omitted: %s", raw)
	}
}

func TestOrientedDimsMatchPreview(t *testing.T) {
	cfg, st := newRepo(t)
	p := fixture(t, "orient6.jpg", "rot.jpg")
	recs, err := Add(cfg, st, []string{p}, AddOpts{})
	must(t, err)
	r := recs[0]
	if (r.Width > r.Height) != (r.PW > r.PH) {
		t.Errorf("dims %dx%d inconsistent with preview %dx%d", r.Width, r.Height, r.PW, r.PH)
	}
}

func TestReAddUnionsAlbumsNoReprocess(t *testing.T) {
	cfg, st := newRepo(t)
	p := fixture(t, "landscape.jpg", "p.jpg")
	_, err := Add(cfg, st, []string{p}, AddOpts{Albums: []string{"a"}})
	must(t, err)
	fi, err := os.Stat("assets/gallery/p-thumb.webp")
	must(t, err)
	before := fi.ModTime()
	// Age the file so a rewrite would be visible.
	old := before.Add(-3600e9)
	must(t, os.Chtimes("assets/gallery/p-thumb.webp", old, old))
	_, err = Add(cfg, st, []string{p}, AddOpts{Albums: []string{"b"}})
	must(t, err)
	photos, err := catalog.Load(cfg)
	must(t, err)
	if len(photos) != 1 {
		t.Fatalf("got %d records", len(photos))
	}
	if got := photos[0].Albums; len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("albums = %v", got)
	}
	fi2, _ := os.Stat("assets/gallery/p-thumb.webp")
	if !fi2.ModTime().Equal(old) {
		t.Errorf("thumb re-encoded: %v != %v", fi2.ModTime(), old)
	}
	if _, err := os.Stat("content/albums/b/index.md"); err != nil {
		t.Errorf("album b page missing")
	}
}

func TestAddRejectsReservedAlbum(t *testing.T) {
	cfg, st := newRepo(t)
	p := fixture(t, "landscape.jpg", "p.jpg")
	_, err := Add(cfg, st, []string{p}, AddOpts{Albums: []string{"unassigned"}})
	if err == nil {
		t.Fatal("expected error")
	}
	photos, _ := catalog.Load(cfg)
	if len(photos) != 0 {
		t.Errorf("catalog modified: %v", photos)
	}
	if _, err := os.Stat("assets/gallery/p-thumb.webp"); err == nil {
		t.Errorf("derivative created before validation")
	}
}

func TestLinkRejectsReserved(t *testing.T) {
	cfg, _ := newRepo(t)
	if err := Link(cfg, "unassigned", []string{"x"}); err == nil {
		t.Fatal("expected error")
	}
}

func TestRemoveQueuesS3Delete(t *testing.T) {
	cfg, st := newRepo(t)
	p := fixture(t, "landscape.jpg", "gone.jpg")
	_, err := Add(cfg, st, []string{p}, AddOpts{})
	must(t, err)
	_, err = Remove(cfg, st, []string{"gone"})
	must(t, err)
	photos, _ := catalog.Load(cfg)
	if len(photos) != 0 {
		t.Errorf("record not removed")
	}
	for _, f := range []string{"assets/gallery/gone-thumb.webp", "assets/gallery/gone-preview.webp"} {
		if _, err := os.Stat(f); err == nil {
			t.Errorf("%s still exists", f)
		}
	}
	n, err := Count(".")
	must(t, err)
	if n != 1 {
		t.Errorf("pending = %d", n)
	}
	if _, err := os.Stat(p); err != nil {
		t.Errorf("source original touched")
	}
}

func TestRemoveUnknownIDTouchesNothing(t *testing.T) {
	cfg, st := newRepo(t)
	p := fixture(t, "landscape.jpg", "good.jpg")
	_, err := Add(cfg, st, []string{p}, AddOpts{})
	must(t, err)
	if _, err := Remove(cfg, st, []string{"good", "bogus"}); err == nil {
		t.Fatal("expected error")
	}
	for _, f := range []string{"assets/gallery/good-thumb.webp", "assets/gallery/good-preview.webp"} {
		if _, err := os.Stat(f); err != nil {
			t.Errorf("%s was deleted", f)
		}
	}
	if n, _ := Count("."); n != 0 {
		t.Errorf("pending = %d, want 0", n)
	}
	if photos, _ := catalog.Load(cfg); len(photos) != 1 {
		t.Errorf("catalog changed: %d", len(photos))
	}
}

func TestRemoveAlbumRejectsInvalidSlug(t *testing.T) {
	cfg, st := newRepo(t)
	must(t, os.MkdirAll("content/albums/keep", 0o755))
	for _, slug := range []string{"", ".", "..", "a/b", `a\b`, "unassigned"} {
		if _, _, _, _, err := RemoveAlbum(cfg, st, slug, true); err == nil {
			t.Errorf("slug %q: expected error", slug)
		}
	}
	if _, err := os.Stat("content/albums/keep"); err != nil {
		t.Errorf("albums dir damaged: %v", err)
	}
	if _, err := Add(cfg, st, nil, AddOpts{Albums: []string{".."}}); err == nil {
		t.Error("Add accepted ..")
	}
}

func TestRemoveAlbumPurgeOrphans(t *testing.T) {
	cfg, st := newRepo(t)
	p := fixture(t, "landscape.jpg", "o.jpg")
	_, err := Add(cfg, st, []string{p}, AddOpts{Albums: []string{"Greece 2026"}})
	must(t, err)
	u, o, pg, _, err := RemoveAlbum(cfg, st, "Greece 2026", true)
	must(t, err)
	if u != 1 || o != 1 || pg != 1 {
		t.Errorf("counts %d %d %d", u, o, pg)
	}
	if n, _ := Count("."); n != 1 {
		t.Errorf("pending = %d", n)
	}
	if _, err := os.Stat("assets/gallery/o-thumb.webp"); err == nil {
		t.Error("derivative remains")
	}
}

type upSpy struct{ names []string }

func (u *upSpy) Upload(f, p string) (string, error) {
	u.names = append(u.names, f)
	return "https://cdn/" + f, nil
}
func (u *upSpy) Delete(string) error    { return nil }
func (u *upSpy) DeleteKey(string) error { return nil }
func (u *upSpy) Copy(a, b string) error { return nil }
func (u *upSpy) Key(f string) string    { return f }

func TestAddSlugifiesID(t *testing.T) {
	cfg, st := newRepo(t)
	p := fixture(t, "landscape.jpg", "DSC 0001.JPG")
	recs, err := Add(cfg, st, []string{p}, AddOpts{})
	must(t, err)
	r := recs[0]
	if r.ID != "dsc-0001" || r.Thumb != "gallery/dsc-0001-thumb.webp" {
		t.Fatalf("bad record %+v", r)
	}
	if _, err := os.Stat("assets/gallery/dsc-0001-thumb.webp"); err != nil {
		t.Error(err)
	}
}

func TestAddUploadFilenameIsSlugPlusLowerExt(t *testing.T) {
	cfg, _ := newRepo(t)
	p := fixture(t, "landscape.jpg", "DSC 0001.JPG")
	sp := &upSpy{}
	_, err := Add(cfg, sp, []string{p}, AddOpts{})
	must(t, err)
	if len(sp.names) != 1 || sp.names[0] != "dsc-0001.jpg" {
		t.Fatalf("upload names %v", sp.names)
	}
	if slugifyName("---") != "photo" {
		t.Error("fallback")
	}
}

// A brand-new site has neither assets/gallery nor data/ yet; add creates them.
func TestAddCreatesMissingDirs(t *testing.T) {
	cfg, st := newRepo(t)
	must(t, os.RemoveAll("assets"))
	must(t, os.RemoveAll("data"))
	src := fixture(t, "landscape.jpg", "fresh.jpg")
	if _, err := Add(cfg, st, []string{src}, AddOpts{}); err != nil {
		t.Fatalf("Add on a fresh site: %v", err)
	}
	for _, p := range []string{cfg.PhotosIndex, filepath.Join(cfg.AssetsDir, "fresh-thumb.webp")} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("missing %s: %v", p, err)
		}
	}
}
