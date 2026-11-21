package content

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PrzemekMalak/contact-sheet/tool/internal/catalog"
	"github.com/PrzemekMalak/contact-sheet/tool/internal/config"
)

func newCfg(t *testing.T) *config.Config {
	d := t.TempDir()
	return &config.Config{AlbumsDir: filepath.Join(d, "albums"), TagsDir: filepath.Join(d, "tags")}
}

func read(t *testing.T, p string) string {
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestEnsureAlbumCreatesPage(t *testing.T) {
	cfg := newCfg(t)
	if err := EnsureAlbum(cfg, "greece-2026", "", "gallery/x-thumb.webp"); err != nil {
		t.Fatal(err)
	}
	got := read(t, filepath.Join(cfg.AlbumsDir, "greece-2026", "index.md"))
	for _, w := range []string{"---\ntitle: \"Greece 2026\"\n", "\ngallery: \"greece-2026\"\n", "\ncover: \"gallery/x-thumb.webp\"\n---\n\n"} {
		if !strings.Contains(got, w) {
			t.Errorf("missing %q in %q", w, got)
		}
	}
	if !strings.HasSuffix(got, "---\n\n") {
		t.Errorf("bad suffix: %q", got)
	}
}

func TestEnsureAlbumNoCoverOmitsKey(t *testing.T) {
	cfg := newCfg(t)
	if err := EnsureAlbum(cfg, "a", "My \"Title\"", ""); err != nil {
		t.Fatal(err)
	}
	got := read(t, filepath.Join(cfg.AlbumsDir, "a", "index.md"))
	if strings.Contains(got, "cover:") {
		t.Errorf("unexpected cover: %q", got)
	}
	if !strings.Contains(got, `title: "My \"Title\""`) {
		t.Errorf("escaping: %q", got)
	}
}

func TestEnsureAlbumExistingUntouched(t *testing.T) {
	cfg := newCfg(t)
	p := filepath.Join(cfg.AlbumsDir, "a", "index.md")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("custom body"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := EnsureAlbum(cfg, "a", "T", "c"); err != nil {
		t.Fatal(err)
	}
	if read(t, p) != "custom body" {
		t.Error("overwritten")
	}
}

func TestSetCoverUpdatesExisting(t *testing.T) {
	cfg := newCfg(t)
	if err := EnsureAlbum(cfg, "a", "", ""); err != nil {
		t.Fatal(err)
	}
	if err := SetCover(cfg, "a", "g/new.webp"); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(cfg.AlbumsDir, "a", "index.md")
	got := read(t, p)
	if !strings.Contains(got, "cover: \"g/new.webp\"\n---\n") || !strings.Contains(got, `title: "A"`) || !strings.Contains(got, `gallery: "a"`) {
		t.Errorf("got %q", got)
	}
	if err := SetCover(cfg, "a", "g/other.webp"); err != nil {
		t.Fatal(err)
	}
	got = read(t, p)
	if strings.Count(got, "cover:") != 1 || !strings.Contains(got, "g/other.webp") {
		t.Errorf("replace failed %q", got)
	}
	if err := SetCover(cfg, "missing", "x"); err == nil {
		t.Error("want error for missing album")
	}
}

func TestRebuildTagStubsCreatesOnePerTag(t *testing.T) {
	cfg := newCfg(t)
	if err := os.MkdirAll(cfg.TagsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfg.TagsDir, "old.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfg.TagsDir, "_index.md"), []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	photos := []catalog.Record{{Tags: []string{"sea", "sun"}}, {Tags: []string{"sea"}}, {Tags: []string{"city"}}}
	if err := RebuildTagStubs(cfg, photos); err != nil {
		t.Fatal(err)
	}
	if got := read(t, filepath.Join(cfg.TagsDir, "sea.md")); got != "---\ntitle: \"sea\"\ntag: \"sea\"\n---\n" {
		t.Errorf("sea: %q", got)
	}
	ents, _ := os.ReadDir(cfg.TagsDir)
	var names []string
	for _, e := range ents {
		names = append(names, e.Name())
	}
	if strings.Join(names, ",") != "_index.md,city.md,sea.md,sun.md" {
		t.Errorf("files: %v", names)
	}
	if read(t, filepath.Join(cfg.TagsDir, "_index.md")) != "keep" {
		t.Error("_index changed")
	}
}

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"FE 400mm F4.5 GM OSS + 1.4X Teleconverter": "fe-400mm-f4-5-gm-oss-1-4x-teleconverter",
		"SONY ILCE-6700": "sony-ilce-6700",
		"***":            "",
	}
	for in, want := range cases {
		if got := Slugify(in); got != want {
			t.Errorf("Slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRebuildGearStubsCreatesPerGear(t *testing.T) {
	cfg := newCfg(t)
	photos := []catalog.Record{
		{ID: "a", Exif: catalog.Exif{"camera": "SONY ILCE-6700", "lens": "FE 400mm"}},
		{ID: "b", Exif: catalog.Exif{"camera": "SONY ILCE-6700", "lens": "E PZ 16-50mm"}},
	}
	if err := RebuildGearStubs(cfg, photos); err != nil {
		t.Fatal(err)
	}
	for _, slug := range []string{"sony-ilce-6700", "fe-400mm", "e-pz-16-50mm"} {
		if _, err := os.Stat(filepath.Join(gearDir(cfg), slug, "index.md")); err != nil {
			t.Errorf("missing gear page %q: %v", slug, err)
		}
	}
	cam := read(t, filepath.Join(gearDir(cfg), "sony-ilce-6700", "index.md"))
	if !strings.Contains(cam, "gearType: \"camera\"") || !strings.Contains(cam, "gear: \"SONY ILCE-6700\"") {
		t.Errorf("camera front matter wrong: %q", cam)
	}
}

func TestRebuildGearStubsPrunesStale(t *testing.T) {
	cfg := newCfg(t)
	g := gearDir(cfg)
	if err := os.MkdirAll(filepath.Join(g, "old-lens"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(g, "old-lens", "index.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(g, "_index.md"), []byte("idx"), 0o644); err != nil {
		t.Fatal(err)
	}
	photos := []catalog.Record{{ID: "a", Exif: catalog.Exif{"lens": "FE 400mm"}}}
	if err := RebuildGearStubs(cfg, photos); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(g, "old-lens")); !os.IsNotExist(err) {
		t.Errorf("stale gear page not pruned")
	}
	if _, err := os.Stat(filepath.Join(g, "_index.md")); err != nil {
		t.Errorf("_index.md should survive: %v", err)
	}
}

func TestRebuildGearStubsSkipsEmpty(t *testing.T) {
	cfg := newCfg(t)
	photos := []catalog.Record{
		{ID: "a", Exif: catalog.Exif{}},
		{ID: "b", Exif: catalog.Exif{"lens": "***"}},
	}
	if err := RebuildGearStubs(cfg, photos); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(gearDir(cfg))
	for _, e := range entries {
		if e.IsDir() {
			t.Errorf("unexpected gear dir %q", e.Name())
		}
	}
}

func TestRebuildGearStubsEscapesYAML(t *testing.T) {
	cfg := newCfg(t)
	photos := []catalog.Record{{ID: "a", Exif: catalog.Exif{"lens": `he said "hi"`}}}
	if err := RebuildGearStubs(cfg, photos); err != nil {
		t.Fatal(err)
	}
	got := read(t, filepath.Join(gearDir(cfg), "he-said-hi", "index.md"))
	if !strings.Contains(got, `\"hi\"`) {
		t.Errorf("YAML not escaped: %q", got)
	}
}

func TestRebuildGearStubsCreatesCombo(t *testing.T) {
	cfg := newCfg(t)
	photos := []catalog.Record{{ID: "a", Exif: catalog.Exif{"camera": "SONY ILCE-6700", "lens": "FE 400mm"}}}
	if err := RebuildGearStubs(cfg, photos); err != nil {
		t.Fatal(err)
	}
	got := read(t, filepath.Join(gearDir(cfg), "sony-ilce-6700-fe-400mm", "index.md"))
	for _, want := range []string{
		`gearType: "combo"`,
		`camera: "SONY ILCE-6700"`,
		`lens: "FE 400mm"`,
		`title: "SONY ILCE-6700 + FE 400mm"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("combo page missing %q: %q", want, got)
		}
	}
}

func TestRebuildGearStubsNoComboWhenPartial(t *testing.T) {
	cfg := newCfg(t)
	photos := []catalog.Record{
		{ID: "a", Exif: catalog.Exif{"camera": "SONY ILCE-6700"}},
		{ID: "b", Exif: catalog.Exif{"lens": "FE 400mm"}},
	}
	if err := RebuildGearStubs(cfg, photos); err != nil {
		t.Fatal(err)
	}
	g := gearDir(cfg)
	for _, slug := range []string{"sony-ilce-6700", "fe-400mm"} {
		if _, err := os.Stat(filepath.Join(g, slug, "index.md")); err != nil {
			t.Errorf("missing gear page %q: %v", slug, err)
		}
	}
	if _, err := os.Stat(filepath.Join(g, "sony-ilce-6700-fe-400mm")); !os.IsNotExist(err) {
		t.Errorf("unexpected combo dir")
	}
	n := 0
	entries, _ := os.ReadDir(g)
	for _, e := range entries {
		if e.IsDir() {
			n++
		}
	}
	if n != 2 {
		t.Errorf("gear dirs = %d, want 2", n)
	}
}

func TestRebuildGearStubsPrunesStaleCombo(t *testing.T) {
	cfg := newCfg(t)
	g := gearDir(cfg)
	if err := os.MkdirAll(filepath.Join(g, "old-combo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(g, "old-combo", "index.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	photos := []catalog.Record{{ID: "a", Exif: catalog.Exif{"camera": "SONY ILCE-6700", "lens": "FE 400mm"}}}
	if err := RebuildGearStubs(cfg, photos); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(g, "old-combo")); !os.IsNotExist(err) {
		t.Errorf("stale combo not pruned")
	}
}

func TestGearTitle(t *testing.T) {
	cameras := map[string]string{
		"NIKON CORPORATION NIKON Z 8":        "Nikon Z 8",
		"SONY ILCE-7M5":                      "Sony ILCE-7M5",
		"Canon Canon EOS R5":                 "Canon EOS R5",
		"OLYMPUS IMAGING CORP.  E-M1":        "Olympus E-M1",
		"FUJIFILM X-T5":                      "Fujifilm X-T5",
		"Panasonic DC-S1M2":                  "Panasonic DC-S1M2",
		"ZWO":                                "ZWO",
		"DJI FC3582":                         "DJI FC3582",
		"NIKON Z 8":                          "Nikon Z 8",
		"  ":                                 "",
		"RICOH IMAGING COMPANY, LTD. GR III": "Ricoh GR III",
		"RICOH IMAGING COMPANY, LTD. PENTAX K-3 Mark III": "Pentax K-3 Mark III",
		"Leica Camera AG LEICA Q2":                        "Leica Q2",
	}
	for raw, want := range cameras {
		got := GearTitle(raw, "camera")
		if got != want {
			t.Errorf("GearTitle(%q, camera) = %q, want %q", raw, got, want)
		}
		if again := GearTitle(got, "camera"); again != got {
			t.Errorf("GearTitle not idempotent: %q -> %q -> %q", raw, got, again)
		}
	}
	const lens = "FE 100-400mm F4.5-5.6 GM OSS"
	if got := GearTitle(lens, "lens"); got != lens {
		t.Errorf("GearTitle(lens) = %q, want unchanged", got)
	}
}

func writeStub(t *testing.T, cfg *config.Config, slug, body string) string {
	t.Helper()
	p := filepath.Join(gearDir(cfg), slug, "index.md")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRebuildGearStubsCleansCameraTitle(t *testing.T) {
	cfg := newCfg(t)
	photos := []catalog.Record{{ID: "a", Exif: catalog.Exif{"camera": "NIKON CORPORATION NIKON Z 8"}}}
	if err := RebuildGearStubs(cfg, photos); err != nil {
		t.Fatal(err)
	}
	got := read(t, filepath.Join(gearDir(cfg), "nikon-corporation-nikon-z-8", "index.md"))
	if !strings.Contains(got, "title: \"Nikon Z 8\"") || !strings.Contains(got, "gear: \"NIKON CORPORATION NIKON Z 8\"") {
		t.Errorf("stub = %q", got)
	}
}

const oldSonyStub = "---\ntitle: \"SONY ILCE-6700\"\nslug: \"sony-ilce-6700\"\ngear: \"SONY ILCE-6700\"\ngearType: \"camera\"\n---\n"

var sonyPhotos = []catalog.Record{{ID: "a", Exif: catalog.Exif{"camera": "SONY ILCE-6700"}}}

func TestRebuildGearStubsRefreshesUntouchedTitle(t *testing.T) {
	cfg := newCfg(t)
	p := writeStub(t, cfg, "sony-ilce-6700", oldSonyStub)
	if err := RebuildGearStubs(cfg, sonyPhotos); err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(oldSonyStub, "title: \"SONY ILCE-6700\"", "title: \"Sony ILCE-6700\"", 1)
	if got := read(t, p); got != want {
		t.Errorf("stub = %q, want %q", got, want)
	}
}

func TestRebuildGearStubsKeepsEditedTitle(t *testing.T) {
	cfg := newCfg(t)
	edited := strings.Replace(oldSonyStub, "title: \"SONY ILCE-6700\"", "title: \"Sony A6700\"", 1)
	p := writeStub(t, cfg, "sony-ilce-6700", edited)
	if err := RebuildGearStubs(cfg, sonyPhotos); err != nil {
		t.Fatal(err)
	}
	if got := read(t, p); got != edited {
		t.Errorf("edited stub changed: %q", got)
	}
}

func TestRebuildGearStubsKeepsUnparseableStub(t *testing.T) {
	cfg := newCfg(t)
	odd := "---\ntitle: SONY ILCE-6700\nslug: \"sony-ilce-6700\"\ngear: \"SONY ILCE-6700\"\ngearType: \"camera\"\nweight: 1\n---\n"
	p := writeStub(t, cfg, "sony-ilce-6700", odd)
	if err := RebuildGearStubs(cfg, sonyPhotos); err != nil {
		t.Fatal(err)
	}
	if got := read(t, p); got != odd {
		t.Errorf("unparseable stub changed: %q", got)
	}
}

func TestRebuildGearStubsRefreshIdempotent(t *testing.T) {
	cfg := newCfg(t)
	p := writeStub(t, cfg, "sony-ilce-6700", oldSonyStub)
	if err := RebuildGearStubs(cfg, sonyPhotos); err != nil {
		t.Fatal(err)
	}
	first := read(t, p)
	if err := RebuildGearStubs(cfg, sonyPhotos); err != nil {
		t.Fatal(err)
	}
	if second := read(t, p); second != first {
		t.Errorf("second run changed stub: %q -> %q", first, second)
	}
}

func writeAlbum(t *testing.T, cfg *config.Config, slug, page string) {
	t.Helper()
	p := filepath.Join(cfg.AlbumsDir, slug, "index.md")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(page), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSetTitleRoundTrip(t *testing.T) {
	cfg := newCfg(t)
	writeAlbum(t, cfg, "trip", "---\ntitle: \"Trip\"\ndate: 2026-01-01\ngallery: \"trip\"\ncover: \"gallery/a-thumb.webp\"\n---\n\nBody text\n")
	title := `Ice "land" \ 2026`
	if err := SetTitle(cfg, "trip", title); err != nil {
		t.Fatal(err)
	}
	got, err := AlbumTitle(cfg, "trip")
	if err != nil || got != title {
		t.Fatalf("got %q, %v; want %q", got, err, title)
	}
	b, _ := os.ReadFile(filepath.Join(cfg.AlbumsDir, "trip", "index.md"))
	for _, keep := range []string{"gallery: \"trip\"", "cover: \"gallery/a-thumb.webp\"", "date: 2026-01-01", "Body text"} {
		if !strings.Contains(string(b), keep) {
			t.Errorf("page lost %q:\n%s", keep, b)
		}
	}
	if strings.Count(string(b), "title:") != 1 {
		t.Errorf("want one title line:\n%s", b)
	}
}

func TestAlbumTitleHandWritten(t *testing.T) {
	cfg := newCfg(t)
	cases := map[string]string{
		"single": "title: 'It''s here'",
		"plain":  "title: Plain words",
		"none":   "gallery: \"none\"",
	}
	want := map[string]string{"single": "It's here", "plain": "Plain words", "none": ""}
	for slug, line := range cases {
		writeAlbum(t, cfg, slug, "---\n"+line+"\n---\n")
		got, err := AlbumTitle(cfg, slug)
		if err != nil || got != want[slug] {
			t.Errorf("%s: got %q, %v; want %q", slug, got, err, want[slug])
		}
	}
	if got, err := AlbumTitle(cfg, "missing"); err != nil || got != "" {
		t.Errorf("missing page: got %q, %v", got, err)
	}
}

func TestSetTitleCreatesMissingPage(t *testing.T) {
	cfg := newCfg(t)
	if err := SetTitle(cfg, "new", "New Album"); err != nil {
		t.Fatal(err)
	}
	if got, _ := AlbumTitle(cfg, "new"); got != "New Album" {
		t.Fatalf("got %q", got)
	}
}

func TestSetTitleCRLF(t *testing.T) {
	cfg := newCfg(t)
	writeAlbum(t, cfg, "win", "---\r\ntitle: \"Old\"\r\ngallery: \"win\"\r\n---\r\n\r\nText\r\n")
	if err := SetTitle(cfg, "win", "New"); err != nil {
		t.Fatal(err)
	}
	if got, _ := AlbumTitle(cfg, "win"); got != "New" {
		t.Fatalf("got %q", got)
	}
	b, _ := os.ReadFile(filepath.Join(cfg.AlbumsDir, "win", "index.md"))
	if strings.Count(string(b), "\r\n") != strings.Count(string(b), "\n") {
		t.Errorf("line endings mixed:\n%q", b)
	}
}
