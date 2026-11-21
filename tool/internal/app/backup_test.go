package app

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/PrzemekMalak/contact-sheet/tool/internal/catalog"
)

func TestBackupDownloadsIncrementally(t *testing.T) {
	cfg, _ := newRepo(t)
	var hits, heads atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			heads.Add(1)
		} else {
			hits.Add(1)
		}
		switch r.URL.Path {
		case "/originals/a.jpg":
			w.Write([]byte("AAAA"))
		case "/originals/b.jpg":
			w.Write([]byte("BB"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	must(t, catalog.Save(cfg, []catalog.Record{
		{ID: "a", Original: srv.URL + "/originals/a.jpg"},
		{ID: "b", Original: srv.URL + "/originals/b.jpg"},
		{ID: "gone", Original: srv.URL + "/originals/gone.jpg"},
		{ID: "local", Original: "gallery/local-preview.webp"}, // local mode: lives in git
	}))
	dest := filepath.Join(t.TempDir(), "backup")

	res, err := Backup(cfg, dest, BackupOpts{Client: srv.Client(), Jobs: 2})
	must(t, err)
	if res.Downloaded != 2 || res.Skipped != 0 || res.NotRemote != 1 || len(res.Failed) != 1 {
		t.Fatalf("first run: %+v", res)
	}
	if !strings.Contains(res.Failed[0], "gone") {
		t.Fatalf("failure should name the photo: %v", res.Failed)
	}
	if b, _ := os.ReadFile(filepath.Join(dest, "a.jpg")); string(b) != "AAAA" {
		t.Fatalf("a.jpg = %q", b)
	}
	if _, err := os.Stat(filepath.Join(dest, "gone.jpg")); !os.IsNotExist(err) {
		t.Fatal("failed download must not leave a file")
	}
	if matches, _ := filepath.Glob(filepath.Join(dest, "*.part")); len(matches) != 0 {
		t.Fatalf("leftover partial files: %v", matches)
	}
	snap, err := os.ReadFile(filepath.Join(dest, "photos.json"))
	must(t, err)
	if !strings.Contains(string(snap), `"id": "a"`) {
		t.Fatalf("catalog snapshot missing: %s", snap)
	}

	// Second run: existing files are checked with a HEAD request (same size:
	// skipped) and not downloaded again.
	hits.Store(0)
	heads.Store(0)
	res, err = Backup(cfg, dest, BackupOpts{Client: srv.Client(), Jobs: 2})
	must(t, err)
	if res.Downloaded != 0 || res.Skipped != 2 || res.Replaced != 0 {
		t.Fatalf("second run: %+v", res)
	}
	if n := hits.Load(); n != 1 { // only the still-missing "gone"
		t.Fatalf("second run made %d downloads, want 1", n)
	}
	if n := heads.Load(); n != 2 {
		t.Fatalf("second run made %d HEAD requests, want 2", n)
	}
}

func TestBackupKeepsExtraFiles(t *testing.T) {
	cfg, _ := newRepo(t)
	must(t, catalog.Save(cfg, []catalog.Record{}))
	dest := t.TempDir()
	must(t, os.WriteFile(filepath.Join(dest, "old.jpg"), []byte("x"), 0o644))
	res, err := Backup(cfg, dest, BackupOpts{})
	must(t, err)
	if res.Extra != 1 {
		t.Fatalf("extra = %d, want 1", res.Extra)
	}
	if _, err := os.Stat(filepath.Join(dest, "old.jpg")); err != nil {
		t.Fatal("backup must never delete files")
	}
}

// A different photo can later get the same file name (a photo was deleted and
// another file with that name added). The backup must not keep the old one.
func TestBackupReplacesFileWhoseSizeChanged(t *testing.T) {
	cfg, _ := newRepo(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "6")
		w.Write([]byte("NEWER!"))
	}))
	defer srv.Close()
	must(t, catalog.Save(cfg, []catalog.Record{{ID: "a", Original: srv.URL + "/originals/a.jpg"}}))
	dest := filepath.Join(t.TempDir(), "backup")
	must(t, os.MkdirAll(dest, 0o755))
	must(t, os.WriteFile(filepath.Join(dest, "a.jpg"), []byte("OLD"), 0o644))
	res, err := Backup(cfg, dest, BackupOpts{Client: srv.Client()})
	must(t, err)
	if res.Replaced != 1 || res.Skipped != 0 {
		t.Fatalf("res %+v", res)
	}
	if b, _ := os.ReadFile(filepath.Join(dest, "a.jpg")); string(b) != "NEWER!" {
		t.Fatalf("a.jpg = %q", b)
	}
}

// When the size can't be checked (HEAD fails or has no length), the existing
// file is kept, as before.
func TestBackupKeepsFileWhenSizeUnknown(t *testing.T) {
	cfg, _ := newRepo(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Write([]byte("NEWER!"))
	}))
	defer srv.Close()
	must(t, catalog.Save(cfg, []catalog.Record{{ID: "a", Original: srv.URL + "/originals/a.jpg"}}))
	dest := filepath.Join(t.TempDir(), "backup")
	must(t, os.MkdirAll(dest, 0o755))
	must(t, os.WriteFile(filepath.Join(dest, "a.jpg"), []byte("OLD"), 0o644))
	res, err := Backup(cfg, dest, BackupOpts{Client: srv.Client()})
	must(t, err)
	if res.Skipped != 1 || res.Replaced != 0 || len(res.Failed) != 0 {
		t.Fatalf("res %+v", res)
	}
	if b, _ := os.ReadFile(filepath.Join(dest, "a.jpg")); string(b) != "OLD" {
		t.Fatalf("a.jpg = %q", b)
	}
}
