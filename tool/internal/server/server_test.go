package server

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/PrzemekMalak/contact-sheet/tool/internal/app"
	"github.com/PrzemekMalak/contact-sheet/tool/internal/catalog"
	"github.com/PrzemekMalak/contact-sheet/tool/internal/config"
	"github.com/PrzemekMalak/contact-sheet/tool/internal/store"
)

type spy struct{ deletes int }

func (s *spy) Upload(f, p string) (string, error) { return "", nil }
func (s *spy) Delete(f string) error              { s.deletes++; return nil }
func (s *spy) DeleteKey(k string) error           { s.deletes++; return nil }
func (s *spy) Copy(a, b string) error             { return nil }
func (s *spy) Key(f string) string                { return f }

var _ store.Store = (*spy)(nil)

func setup(t *testing.T, recs ...catalog.Record) (*config.Config, *spy, *Server) {
	t.Helper()
	t.Chdir(t.TempDir())
	toml := "[project]\nassets_dir = \"assets/gallery\"\nphotos_index = \"data/photos.json\"\nalbums_dir = \"content/albums\"\ntags_dir = \"content/tags\"\n[images]\nthumb_max_edge = 100\npreview_max_edge = 200\nwebp_quality = 80\n[s3]\nbucket = \"\"\n"
	must(t, os.WriteFile("contact-sheet.toml", []byte(toml), 0o644))
	for _, d := range []string{"assets/gallery", "content/albums", "content/tags", "data"} {
		must(t, os.MkdirAll(d, 0o755))
	}
	must(t, os.WriteFile("data/photos.json", []byte("[]\n"), 0o644))
	cfg, err := config.Load(".")
	must(t, err)
	must(t, catalog.Save(cfg, recs))
	sp := &spy{}
	srv := New(cfg, sp)
	// Tests aren't in a git repo: report one change unless a test says otherwise.
	srv.Status = func(string) (string, error) { return " M data/photos.json\n", nil }
	return cfg, sp, srv
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func do(s *Server, method, url, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, url, strings.NewReader(body))
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}

func rec(id string, albums, tags []string) catalog.Record {
	return catalog.Record{ID: id, Thumb: "/assets/gallery/" + id + "-thumb.webp", Preview: "/assets/gallery/" + id + "-preview.webp",
		Original: "https://cdn.example/orig/" + id + ".jpg", Albums: albums, Tags: tags}
}

func TestGetPhotosReturnsCatalog(t *testing.T) {
	_, _, s := setup(t, rec("a", []string{"trip"}, []string{"sea"}), rec("b", []string{"home"}, []string{"cat"}))
	w := do(s, "GET", "/api/photos", "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	var out struct {
		Photos []catalog.Record `json:"photos"`
		Albums []string         `json:"albums"`
		Tags   []string         `json:"tags"`
	}
	must(t, json.Unmarshal(w.Body.Bytes(), &out))
	if len(out.Photos) != 2 || strings.Join(out.Albums, ",") != "home,trip" || strings.Join(out.Tags, ",") != "cat,sea" {
		t.Errorf("unexpected: %+v", out)
	}
}

func TestLinkAlbumEndpoint(t *testing.T) {
	cfg, _, s := setup(t, rec("a", nil, nil))
	w := do(s, "POST", "/api/photos/a/albums", `{"albums":["newalbum"]}`)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	ps, _ := catalog.Load(cfg)
	r, _ := catalog.Get(ps, "a")
	if len(r.Albums) != 1 || r.Albums[0] != "newalbum" {
		t.Errorf("albums = %v", r.Albums)
	}
}

func TestPatchUnknownIs404(t *testing.T) {
	_, _, s := setup(t)
	if w := do(s, "PATCH", "/api/photos/nope", `{"caption":"x"}`); w.Code != 404 {
		t.Errorf("code %d", w.Code)
	}
}

func TestPatchCaption(t *testing.T) {
	cfg, _, s := setup(t, rec("a", nil, nil))
	if w := do(s, "PATCH", "/api/photos/a", `{"caption":"hi","featured":true}`); w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	ps, _ := catalog.Load(cfg)
	r, _ := catalog.Get(ps, "a")
	if r.Caption != "hi" || !r.Featured {
		t.Errorf("%+v", r)
	}
}

func TestDeletePhotoDefersS3(t *testing.T) {
	cfg, sp, s := setup(t, rec("a", nil, nil))
	if w := do(s, "DELETE", "/api/photos/a", ""); w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	ps, _ := catalog.Load(cfg)
	if len(ps) != 0 {
		t.Error("record not removed")
	}
	w := do(s, "GET", "/api/status", "")
	var st struct {
		PendingDeletes int `json:"pendingDeletes"`
	}
	must(t, json.Unmarshal(w.Body.Bytes(), &st))
	if st.PendingDeletes != 1 {
		t.Errorf("pending = %d", st.PendingDeletes)
	}
	if sp.deletes != 0 {
		t.Errorf("S3 deletes = %d, want 0", sp.deletes)
	}
}

func TestPublishFailureKeepsQueue(t *testing.T) {
	_, sp, s := setup(t)
	must(t, app.Queue(".", "x.jpg"))
	s.Commit = func(string, string) error { return nil }
	s.Push = func(string) error { return errors.New("push rejected") }
	w := do(s, "POST", "/api/publish", `{"message":"m"}`)
	if w.Code < 500 {
		t.Fatalf("code %d", w.Code)
	}
	if n, _ := app.Count("."); n != 1 {
		t.Errorf("queue = %d, want 1", n)
	}
	if sp.deletes != 0 {
		t.Errorf("deletes = %d", sp.deletes)
	}
}

// Publish doesn't delete originals right away: the host still has to
// deploy, and a failed build or a rollback would link to them. They're
// deleted by a Publish at least app.DeleteGrace later.
func TestPublishSchedulesDeletes(t *testing.T) {
	_, sp, s := setup(t)
	must(t, app.Queue(".", "x.jpg"))
	now := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	s.Now = func() time.Time { return now }
	s.Commit = func(string, string) error { return nil }
	s.Push = func(string) error { return nil }
	w := do(s, "POST", "/api/publish", `{"message":"m"}`)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	if sp.deletes != 0 {
		t.Fatalf("deleted right after publish: %d", sp.deletes)
	}
	var st struct {
		PendingDeletes   int    `json:"pendingDeletes"`
		ScheduledDeletes int    `json:"scheduledDeletes"`
		NextDeleteDue    string `json:"nextDeleteDue"`
	}
	must(t, json.Unmarshal(do(s, "GET", "/api/status", "").Body.Bytes(), &st))
	if st.PendingDeletes != 0 || st.ScheduledDeletes != 1 || !strings.HasPrefix(st.NextDeleteDue, "2026-11-08") {
		t.Errorf("status %+v", st)
	}
	// A week later, the next Publish deletes it.
	now = now.Add(app.DeleteGrace)
	if w := do(s, "POST", "/api/publish", `{"message":"m2"}`); w.Code != 200 || !strings.Contains(w.Body.String(), `"deletedFromS3":1`) {
		t.Fatal(w.Code, w.Body)
	}
	if sp.deletes != 1 {
		t.Errorf("deletes = %d, want 1", sp.deletes)
	}
}

// A photo deleted in the GUI and restored with git before publishing keeps
// its original.
func TestPublishKeepsRestoredOriginal(t *testing.T) {
	cfg, sp, s := setup(t, rec("a", nil, nil))
	must(t, app.Queue(".", "a.jpg")) // queued earlier, then the record came back
	now := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	s.Now = func() time.Time { return now }
	s.Commit = func(string, string) error { return nil }
	s.Push = func(string) error { return nil }
	do(s, "POST", "/api/publish", `{"message":"m"}`)
	now = now.Add(app.DeleteGrace)
	do(s, "POST", "/api/publish", `{"message":"m2"}`)
	if sp.deletes != 0 {
		t.Errorf("deleted an original the catalog uses")
	}
	if keys, _ := app.QueuedKeys("."); len(keys) != 0 {
		t.Errorf("queue = %v", keys)
	}
	_ = cfg
}

func TestAlbumEndpointsValidateSlug(t *testing.T) {
	_, _, srv := setup(t)
	for _, slug := range []string{"unassigned", "..", "a/b"} {
		b, _ := json.Marshal(map[string]string{"slug": slug})
		if w := do(srv, "POST", "/api/albums", string(b)); w.Code != 400 {
			t.Errorf("create %q: %d", slug, w.Code)
		}
	}
	if _, err := os.Stat("content/albums/unassigned"); err == nil {
		t.Error("unassigned dir created")
	}
	if w := do(srv, "PATCH", "/api/albums/unassigned", `{"cover":"x"}`); w.Code != 400 {
		t.Errorf("patch unassigned: %d", w.Code)
	}
	if w := do(srv, "POST", "/api/albums", `{"slug":"greece"}`); w.Code != 200 {
		t.Errorf("valid create: %d %s", w.Code, w.Body)
	}
}

func TestPatchTags(t *testing.T) {
	cfg, _, s := setup(t, rec("a", nil, []string{"old"}))
	// Scrambled input: trimmed, blanks dropped, duplicates removed, order kept.
	if w := do(s, "PATCH", "/api/photos/a", `{"tags":["Birds"," birds ","Birds",""]}`); w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	ps, _ := catalog.Load(cfg)
	r, _ := catalog.Get(ps, "a")
	if got := strings.Join(r.Tags, ","); got != "Birds,birds" {
		t.Fatalf("tags = %q, want %q", got, "Birds,birds")
	}
	// Tag stub pages are rebuilt: new tags created.
	for _, tg := range []string{"Birds", "birds"} {
		if _, err := os.Stat(cfg.TagsDir + "/" + tg + ".md"); err != nil {
			t.Errorf("missing tag stub for %q: %v", tg, err)
		}
	}
}

func TestPublishRunsSync(t *testing.T) {
	cfg, _, s := setup(t, rec("a", nil, []string{"bird"}))
	s.Commit = func(string, string) error { return nil }
	s.Push = func(string) error { return nil }
	// A stale tag stub no photo references — sync must prune it before commit.
	must(t, os.WriteFile(cfg.TagsDir+"/old.md", []byte("---\ntitle: \"old\"\ntag: \"old\"\n---\n"), 0o644))
	if w := do(s, "POST", "/api/publish", `{"message":"m"}`); w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	if _, err := os.Stat(cfg.TagsDir + "/bird.md"); err != nil {
		t.Errorf("sync should have created bird.md before commit: %v", err)
	}
	if _, err := os.Stat(cfg.TagsDir + "/old.md"); !os.IsNotExist(err) {
		t.Errorf("sync should have pruned stale old.md before commit")
	}
}

func writePage(t *testing.T, slug, page string) {
	t.Helper()
	must(t, os.MkdirAll("content/albums/"+slug, 0o755))
	must(t, os.WriteFile("content/albums/"+slug+"/index.md", []byte(page), 0o644))
}

func TestGetPhotosReturnsAlbumTitles(t *testing.T) {
	_, _, s := setup(t, rec("a", []string{"trip", "home"}, nil))
	writePage(t, "trip", "---\ntitle: \"Trip to Iceland\"\ngallery: \"trip\"\n---\n")
	w := do(s, "GET", "/api/photos", "")
	var out struct {
		AlbumTitles map[string]string `json:"albumTitles"`
	}
	must(t, json.Unmarshal(w.Body.Bytes(), &out))
	if out.AlbumTitles["trip"] != "Trip to Iceland" {
		t.Errorf("albumTitles = %v", out.AlbumTitles)
	}
	if _, ok := out.AlbumTitles["home"]; ok {
		t.Errorf("home has no page, want no entry: %v", out.AlbumTitles)
	}
}

func TestPatchAlbumTitle(t *testing.T) {
	cfg, _, s := setup(t, rec("a", []string{"trip"}, nil))
	writePage(t, "trip", "---\ntitle: \"Trip\"\ngallery: \"trip\"\ncover: \"x\"\n---\n")
	w := do(s, "PATCH", "/api/albums/trip", `{"title":"Iceland 2026"}`)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	b, _ := os.ReadFile("content/albums/trip/index.md")
	if !strings.Contains(string(b), `title: "Iceland 2026"`) || !strings.Contains(string(b), `cover: "x"`) {
		t.Errorf("page:\n%s", b)
	}
	_ = cfg
}

func TestPatchAlbumRejectsEmpty(t *testing.T) {
	_, _, s := setup(t, rec("a", []string{"trip"}, nil))
	writePage(t, "trip", "---\ntitle: \"Trip\"\n---\n")
	for _, body := range []string{`{"title":"  "}`, `{}`} {
		if w := do(s, "PATCH", "/api/albums/trip", body); w.Code != 400 {
			t.Errorf("%s: want 400, got %d %s", body, w.Code, w.Body)
		}
	}
}

func TestDeleteAlbumUnlinks(t *testing.T) {
	cfg, sp, s := setup(t, rec("a", []string{"trip"}, nil), rec("b", []string{"trip", "home"}, nil))
	writePage(t, "trip", "---\ntitle: \"Trip\"\n---\n")
	w := do(s, "DELETE", "/api/albums/trip", "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	if !strings.Contains(w.Body.String(), `"orphans":1`) || !strings.Contains(w.Body.String(), `"purged":0`) {
		t.Errorf("body %s", w.Body)
	}
	photos, err := catalog.Load(cfg)
	must(t, err)
	if len(photos) != 2 || len(photos[0].Albums)+len(photos[1].Albums) != 1 {
		t.Errorf("catalog: %+v", photos)
	}
	if _, err := os.Stat("content/albums/trip"); !os.IsNotExist(err) {
		t.Errorf("album page still there: %v", err)
	}
	if n, _ := app.Count("."); n != 0 || sp.deletes != 0 {
		t.Errorf("no S3 deletes expected: queued %d, done %d", n, sp.deletes)
	}
}

func TestDeleteAlbumPurgeQueuesOnly(t *testing.T) {
	cfg, sp, s := setup(t, rec("a", []string{"trip"}, nil), rec("b", []string{"trip", "home"}, nil))
	writePage(t, "trip", "---\ntitle: \"Trip\"\n---\n")
	w := do(s, "DELETE", "/api/albums/trip?purge=true", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"purged":1`) {
		t.Fatal(w.Code, w.Body)
	}
	photos, err := catalog.Load(cfg)
	must(t, err)
	if len(photos) != 1 || photos[0].ID != "b" {
		t.Errorf("catalog: %+v", photos)
	}
	if n, _ := app.Count("."); n != 1 || sp.deletes != 0 {
		t.Errorf("want 1 queued and 0 done S3 deletes, got %d queued, %d done", n, sp.deletes)
	}
}

func TestDeleteAlbumReservedSlug(t *testing.T) {
	_, _, s := setup(t)
	if w := do(s, "DELETE", "/api/albums/unassigned", ""); w.Code != 400 {
		t.Errorf("want 400, got %d", w.Code)
	}
}

func TestGetPhotosTitlesForEmptyAlbums(t *testing.T) {
	_, _, s := setup(t)
	writePage(t, "iceland-2026", "---\ntitle: \"Iceland 2026\"\n---\n")
	w := do(s, "GET", "/api/photos", "")
	var out struct {
		AlbumTitles map[string]string `json:"albumTitles"`
	}
	must(t, json.Unmarshal(w.Body.Bytes(), &out))
	if out.AlbumTitles["iceland-2026"] != "Iceland 2026" {
		t.Errorf("albumTitles = %v", out.AlbumTitles)
	}
}

func TestPatchAlbumCoverStillWorks(t *testing.T) {
	_, _, s := setup(t, rec("a", []string{"trip"}, nil))
	writePage(t, "trip", "---\ntitle: \"Trip\"\n---\n")
	if w := do(s, "PATCH", "/api/albums/trip", `{"cover":"gallery/a-thumb.webp"}`); w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	b, _ := os.ReadFile("content/albums/trip/index.md")
	if !strings.Contains(string(b), `cover: "gallery/a-thumb.webp"`) || !strings.Contains(string(b), `title: "Trip"`) {
		t.Errorf("page:\n%s", b)
	}
}

// A delete queued while Publish runs (after it committed) isn't in that
// commit, so that Publish must not stamp it as published.
func TestPublishStampsOnlyDeletesQueuedBefore(t *testing.T) {
	_, _, s := setup(t)
	must(t, app.Queue(".", "x.jpg"))
	s.Commit = func(string, string) error { return nil }
	s.Push = func(string) error { return app.Queue(".", "late.jpg") }
	if w := do(s, "POST", "/api/publish", `{"message":"m"}`); w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	if n, _ := app.Count("."); n != 1 {
		t.Errorf("unpublished = %d, want 1 (late.jpg)", n)
	}
	if n, _, _ := app.Scheduled("."); n != 1 {
		t.Errorf("scheduled = %d, want 1 (x.jpg)", n)
	}
}

func TestStatusLeavesOutRestoredPhotos(t *testing.T) {
	_, _, s := setup(t, rec("a", nil, nil))
	s.Status = func(string) (string, error) { return "", nil }
	must(t, app.Queue(".", "a.jpg")) // the photo is back in the catalog
	var st struct {
		PendingDeletes int `json:"pendingDeletes"`
	}
	must(t, json.Unmarshal(do(s, "GET", "/api/status", "").Body.Bytes(), &st))
	if st.PendingDeletes != 0 {
		t.Errorf("pending = %d, want 0", st.PendingDeletes)
	}
}

// With nothing to commit, Publish still pushes and deletes what is due.
func TestPublishWithNothingToCommitDeletesDue(t *testing.T) {
	_, sp, s := setup(t)
	now := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	must(t, app.Queue(".", "x.jpg"))
	must(t, app.MarkPublished(".", now, []string{"x.jpg"}))
	s.Now = func() time.Time { return now.Add(app.DeleteGrace) }
	s.Status = func(string) (string, error) { return "", nil }
	committed, pushed := false, false
	s.Commit = func(string, string) error { committed = true; return errors.New("nothing to commit") }
	s.Push = func(string) error { pushed = true; return nil }
	var st struct {
		DueDeletes int `json:"dueDeletes"`
	}
	must(t, json.Unmarshal(do(s, "GET", "/api/status", "").Body.Bytes(), &st))
	if st.DueDeletes != 1 {
		t.Fatalf("dueDeletes = %d, want 1", st.DueDeletes)
	}
	w := do(s, "POST", "/api/publish", `{"message":"m"}`)
	if w.Code != 200 || committed || !pushed || sp.deletes != 1 {
		t.Fatalf("code %d committed %v pushed %v deletes %d: %s", w.Code, committed, pushed, sp.deletes, w.Body)
	}
}
