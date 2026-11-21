package theme_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PrzemekMalak/contact-sheet/tool/internal/theme"
)

type entry struct {
	name, body string
	typ        byte
}

func archive(t *testing.T, entries ...entry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		typ := e.typ
		if typ == 0 {
			typ = tar.TypeReg
		}
		h := &tar.Header{Name: e.name, Typeflag: typ, Mode: 0o644, Size: int64(len(e.body))}
		if typ == tar.TypeDir {
			h.Mode, h.Size = 0o755, 0
		}
		if typ == tar.TypeSymlink {
			h.Linkname, h.Size = e.body, 0
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if typ == tar.TypeReg {
			if _, err := tw.Write([]byte(e.body)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func good(t *testing.T, version string) []byte {
	return archive(t,
		entry{name: "./", typ: tar.TypeDir},
		entry{name: "./VERSION", body: version + "\n"},
		entry{name: "./layouts/", typ: tar.TypeDir},
		entry{name: "./layouts/index.html", body: "home " + version},
		entry{name: "./theme.toml", body: "name = \"Contact Sheet\"\n"},
	)
}

func read(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestInstallFromLocalFileAndUpdate(t *testing.T) {
	site := t.TempDir()
	src := filepath.Join(t.TempDir(), "theme.tar.gz")
	if err := os.WriteFile(src, good(t, "v0.1.0"), 0o644); err != nil {
		t.Fatal(err)
	}
	v, err := theme.Install(site, src)
	if err != nil {
		t.Fatal(err)
	}
	if v != "v0.1.0" {
		t.Errorf("version = %q", v)
	}
	dir := filepath.Join(site, "themes", "contact-sheet")
	if got := read(t, filepath.Join(dir, "layouts", "index.html")); got != "home v0.1.0" {
		t.Errorf("layout = %q", got)
	}
	// A stale file from the old version must not survive an update.
	if err := os.WriteFile(filepath.Join(dir, "layouts", "stale.html"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, good(t, "v0.2.0"), 0o644); err != nil {
		t.Fatal(err)
	}
	if v, err = theme.Install(site, src); err != nil || v != "v0.2.0" {
		t.Fatalf("update: %q, %v", v, err)
	}
	if got, _ := theme.InstalledVersion(site); got != "v0.2.0" {
		t.Errorf("InstalledVersion = %q", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "layouts", "stale.html")); !os.IsNotExist(err) {
		t.Error("stale file from the previous theme version survived the update")
	}
	entries, _ := os.ReadDir(filepath.Join(site, "themes"))
	if len(entries) != 1 {
		t.Errorf("leftovers in themes/: %v", entries)
	}
}

func TestInstallFromHTTP(t *testing.T) {
	body := good(t, "v1.0.0")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(body)
	}))
	defer srv.Close()
	if v, err := theme.Install(t.TempDir(), srv.URL+"/contact-sheet-theme.tar.gz"); err != nil || v != "v1.0.0" {
		t.Fatalf("got %q, %v", v, err)
	}
}

func TestInstallHTTPErrorKeepsExistingTheme(t *testing.T) {
	site := t.TempDir()
	src := filepath.Join(t.TempDir(), "theme.tar.gz")
	os.WriteFile(src, good(t, "v0.1.0"), 0o644)
	if _, err := theme.Install(site, src); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	if _, err := theme.Install(site, srv.URL+"/missing.tar.gz"); err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("err = %v, want a 404 error", err)
	}
	if v, _ := theme.InstalledVersion(site); v != "v0.1.0" {
		t.Errorf("existing theme lost after failed update: version %q", v)
	}
}

func TestInstallRejectsUnsafeArchives(t *testing.T) {
	cases := map[string][]byte{
		"parent path": archive(t, entry{name: "../evil.txt", body: "x"}),
		"absolute":    archive(t, entry{name: "/tmp/evil.txt", body: "x"}),
		"symlink":     archive(t, entry{name: "link", body: "/etc/passwd", typ: tar.TypeSymlink}),
		"no VERSION":  archive(t, entry{name: "layouts/index.html", body: "x"}),
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			site := t.TempDir()
			src := filepath.Join(t.TempDir(), "theme.tar.gz")
			os.WriteFile(src, data, 0o644)
			if _, err := theme.Install(site, src); err == nil {
				t.Fatal("unsafe archive accepted")
			}
			if _, err := os.Stat(filepath.Join(site, "themes", "contact-sheet")); !os.IsNotExist(err) {
				t.Error("theme dir created from a rejected archive")
			}
		})
	}
}

func TestSource(t *testing.T) {
	t.Setenv("CONTACT_SHEET_THEME", "")
	if got := theme.Source("v1.2.3"); got != "https://github.com/PrzemekMalak/contact-sheet/releases/download/v1.2.3/contact-sheet-theme.tar.gz" {
		t.Errorf("tagged: %s", got)
	}
	for _, v := range []string{"", "dev", "v0.0.0-20261119-abc"} {
		if got := theme.Source(v); got != "https://github.com/PrzemekMalak/contact-sheet/releases/latest/download/contact-sheet-theme.tar.gz" {
			t.Errorf("Source(%q) = %s, want latest", v, got)
		}
	}
	t.Setenv("CONTACT_SHEET_THEME", "/tmp/x.tar.gz")
	if got := theme.Source("v1.2.3"); got != "/tmp/x.tar.gz" {
		t.Errorf("override ignored: %s", got)
	}
}
