package e2e_test

import (
	"fmt"
	"html"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	in, err := os.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		t.Fatal(err)
	}
}

func runCmd(t *testing.T, dir, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v failed: %v\n%s", name, args, err, out)
	}
	return string(out)
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestE2EHugoParity builds the photo binary, drives it against an empty site
// using exampleSite's config and the repo's theme, renders with hugo, and
// asserts on the generated HTML.
func TestE2EHugoParity(t *testing.T) {
	if _, err := exec.LookPath("hugo"); err != nil {
		t.Skip("hugo not installed")
	}
	toolDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	repo := filepath.Dir(toolDir)
	bin := filepath.Join(t.TempDir(), "photo")
	runCmd(t, toolDir, "go", "build", "-o", bin, "./cmd/photo")

	// The site under test is exampleSite's config with no photos or content of
	// its own: the theme supplies layouts, assets and the section pages.
	example := filepath.Join(repo, "exampleSite")
	site := t.TempDir()
	themes := t.TempDir()
	if err := os.Symlink(repo, filepath.Join(themes, "contact-sheet")); err != nil {
		t.Fatal(err)
	}
	// The theme is the repo root; expose it as <themesDir>/contact-sheet. Pin the homepage to the
	// "daily" album grid so the assertions below do not depend on the
	// example's own homeMode/homeAlbum choice.
	hugoText := readFile(t, filepath.Join(example, "hugo.toml"))
	hugoText = regexp.MustCompile(`(?m)^themesDir\s*=.*$`).ReplaceAllString(hugoText, fmt.Sprintf("themesDir = %q", themes))
	hugoText = regexp.MustCompile(`(?m)^\s*homeMode\s*=.*$`).ReplaceAllString(hugoText, `  homeMode = "album"`)
	hugoText = regexp.MustCompile(`(?m)^\s*homeAlbum\s*=.*$`).ReplaceAllString(hugoText, `  homeAlbum = "daily"`)
	if err := os.WriteFile(filepath.Join(site, "hugo.toml"), []byte(hugoText), 0o644); err != nil {
		t.Fatal(err)
	}
	// exampleSite runs in local mode (empty bucket), so no S3 is touched.
	copyFile(t, filepath.Join(example, "contact-sheet.toml"), filepath.Join(site, "contact-sheet.toml"))

	// Fixtures; file stems become photo ids.
	src := t.TempDir()
	const dated, plain, feat = "xqdated", "xqplain", "xqfeat"
	// full.jpg is an EXIF-only stub (no pixels): splice its EXIF APP1 segment
	// (everything but the trailing EOI) in front of a real image's scan data.
	exifOnly := readFile(t, filepath.Join(toolDir, "internal/xexif/testdata/full.jpg"))
	pixels := readFile(t, filepath.Join(toolDir, "internal/imaging/testdata/landscape.jpg"))
	datedJPEG := exifOnly[:len(exifOnly)-2] + pixels[2:]
	if err := os.WriteFile(filepath.Join(src, dated+".jpg"), []byte(datedJPEG), 0o644); err != nil {
		t.Fatal(err)
	}
	copyFile(t, filepath.Join(toolDir, "internal/imaging/testdata/landscape.jpg"), filepath.Join(src, plain+".jpg"))
	copyFile(t, filepath.Join(toolDir, "internal/imaging/testdata/orient6.jpg"), filepath.Join(src, feat+".jpg"))

	runCmd(t, site, bin, "add", filepath.Join(src, dated+".jpg"), "-a", "daily", "-a", "greece")
	runCmd(t, site, bin, "add", filepath.Join(src, plain+".jpg"), "--tags", "bird")
	runCmd(t, site, bin, "add", filepath.Join(src, feat+".jpg"), "-a", "daily", "--featured")
	runCmd(t, site, bin, "rm", feat)

	runCmd(t, site, "hugo", "--minify")

	page := func(rel string) string { return readFile(t, filepath.Join(site, "public", rel)) }
	has := func(rel, id string, want bool) {
		t.Helper()
		if got := strings.Contains(page(rel), id); got != want {
			t.Errorf("%s contains %q = %v, want %v", rel, id, got, want)
		}
	}
	has("albums/daily/index.html", dated, true)
	has("albums/greece/index.html", dated, true)
	has("albums/unassigned/index.html", plain, true)
	has("albums/unassigned/index.html", dated, false)
	has("tags/bird/index.html", plain, true)
	has("index.html", dated, true)
	if !strings.Contains(page("index.html"), "ph-date") {
		t.Error("homepage missing capture-date label (ph-date)")
	}
	for _, rel := range []string{"index.html", "albums/daily/index.html", "albums/unassigned/index.html", "featured/index.html"} {
		has(rel, feat, false)
	}
	if _, err := os.Stat(filepath.Join(site, "assets", "gallery", feat+"-thumb.webp")); !os.IsNotExist(err) {
		t.Errorf("removed photo derivative still present (err=%v)", err)
	}

	// Gear browse: xqdated's camera/lens (from full.jpg EXIF) become pages.
	// Slugs are the literal expected values — an independent check that the
	// generator, the page URL, and the grid link slug all agree.
	const camSlug = "sony-ilce-6700"
	const lensSlug = "fe-400mm-f4-5-gm-oss-1-4x-teleconverter"
	for _, slug := range []string{camSlug, lensSlug} {
		if _, err := os.Stat(filepath.Join(site, "content", "gear", slug, "index.md")); err != nil {
			t.Errorf("missing generated gear page %q: %v", slug, err)
		}
	}
	has("gear/"+lensSlug+"/index.html", dated, true)
	has("gear/"+camSlug+"/index.html", dated, true)
	home := page("index.html")
	for _, attr := range []string{"data-lens-gear=" + lensSlug, "data-camera-gear=" + camSlug} {
		if !strings.Contains(home, attr) {
			t.Errorf("homepage grid missing %q", attr)
		}
	}
	// Combination (camera x lens) page and the /gear/ index.
	const comboSlug = "sony-ilce-6700-fe-400mm-f4-5-gm-oss-1-4x-teleconverter"
	if _, err := os.Stat(filepath.Join(site, "content", "gear", comboSlug, "index.md")); err != nil {
		t.Errorf("missing generated combo gear page %q: %v", comboSlug, err)
	}
	has("gear/"+comboSlug+"/index.html", dated, true)
	gearIdx := page("gear/index.html")
	for _, link := range []string{"/gear/" + camSlug + "/", "/gear/" + lensSlug + "/", "/gear/" + comboSlug + "/"} {
		if !strings.Contains(gearIdx, link) {
			t.Errorf("gear index missing link %q", link)
		}
	}

	// Display names: the camera title is cleaned by the tool, the index shows
	// icons and thumbnails, and setups are named after their camera and lens.
	const lensName = "FE 400mm F4.5 GM OSS + 1.4X Teleconverter"
	for _, want := range []string{"Sony ILCE-6700", "gear-thumb", "gear-icon", "Sony ILCE-6700 + " + lensName} {
		if !strings.Contains(gearIdx, want) {
			t.Errorf("gear index missing %q", want)
		}
	}
	if strings.Contains(gearIdx, ">SONY ILCE-6700<") {
		t.Error("gear index shows the raw camera name")
	}

	// Gear page headers: type, name, count, and chips to the setups.
	contains := func(rel string, wants ...string) {
		t.Helper()
		got := page(rel)
		for _, w := range wants {
			if !strings.Contains(got, w) {
				t.Errorf("%s missing %q", rel, w)
			}
		}
	}
	contains("gear/"+camSlug+"/index.html", "gear-kind", "Camera", "Sony ILCE-6700", "1 photo", "gear-chip", "/gear/"+comboSlug+"/", lensName)
	contains("gear/"+lensSlug+"/index.html", "Lens", "/gear/"+comboSlug+"/", "Sony ILCE-6700")
	contains("gear/"+comboSlug+"/index.html", "Setup", "/gear/"+camSlug+"/", "/gear/"+lensSlug+"/", "<title>Sony ILCE-6700 + "+lensName)

	// The owner renames the camera; every derived name follows.
	camStub := filepath.Join(site, "content", "gear", camSlug, "index.md")
	setTitle := func(stub, title string) {
		t.Helper()
		b := regexp.MustCompile(`(?m)^title: .*$`).ReplaceAllString(readFile(t, stub), "title: "+strconv.Quote(title))
		if err := os.WriteFile(stub, []byte(b), 0o644); err != nil {
			t.Fatal(err)
		}
		runCmd(t, site, "hugo", "--minify")
	}
	setTitle(camStub, "Sony A6700")
	gearIdx = page("gear/index.html")
	for _, want := range []string{"Sony A6700", "Sony A6700 + " + lensName} {
		if !strings.Contains(gearIdx, want) {
			t.Errorf("gear index after rename missing %q", want)
		}
	}
	contains("gear/"+comboSlug+"/index.html", "<title>Sony A6700 + "+lensName, "<h1>Sony A6700 + "+lensName)

	// The lightbox and the photo page use the display names too.
	attr := func(rel, name string) string {
		t.Helper()
		m := regexp.MustCompile(name + `=(?:"([^"]*)"|'([^']*)'|([^\s>]+))`).FindStringSubmatch(page(rel))
		if m == nil {
			t.Errorf("%s: no %s attribute", rel, name)
			return ""
		}
		return html.UnescapeString(m[1] + m[2] + m[3])
	}
	if got := attr("albums/daily/index.html", "data-camera-name"); got != "Sony A6700" {
		t.Errorf("data-camera-name = %q, want %q", got, "Sony A6700")
	}
	if got := attr("albums/daily/index.html", "data-lens-name"); got != lensName {
		t.Errorf("data-lens-name = %q, want %q", got, lensName)
	}
	contains("p/"+dated+"/index.html", "Sony A6700")

	// A title with quotes and an ampersand stays one well-formed attribute.
	setTitle(camStub, `Sony "A7" & co`)
	if got := attr("albums/daily/index.html", "data-camera-name"); got != `Sony "A7" & co` {
		t.Errorf("data-camera-name = %q, want %q", got, `Sony "A7" & co`)
	}

	// An empty title falls back to the raw EXIF value too.
	setTitle(camStub, "")
	if got := attr("albums/daily/index.html", "data-camera-name"); got != "SONY ILCE-6700" {
		t.Errorf("empty-title data-camera-name = %q, want raw value", got)
	}

	// Without its gear page, a lens falls back to the raw EXIF value.
	if err := os.RemoveAll(filepath.Join(site, "content", "gear", lensSlug)); err != nil {
		t.Fatal(err)
	}
	runCmd(t, site, "hugo", "--minify")
	if got := attr("albums/daily/index.html", "data-lens-name"); got != lensName {
		t.Errorf("fallback data-lens-name = %q, want %q", got, lensName)
	}
}

// TestE2EInit follows the README's quick start (photo init, photo add, hugo)
// in both theme modes, offline: the theme copy comes from an archive packaged
// from this repo, and the Hugo Module is replaced by this repo.
func TestE2EInit(t *testing.T) {
	if _, err := exec.LookPath("hugo"); err != nil {
		t.Skip("hugo not installed")
	}
	toolDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	repo := filepath.Dir(toolDir)
	bin := filepath.Join(t.TempDir(), "photo")
	runCmd(t, toolDir, "go", "build", "-o", bin, "./cmd/photo")

	t.Run("theme copy (default, no Go needed)", func(t *testing.T) {
		archive := filepath.Join(t.TempDir(), "contact-sheet-theme.tar.gz")
		runCmd(t, repo, "bash", "scripts/package-theme.sh", "v9.9.9", archive)
		t.Setenv("CONTACT_SHEET_THEME", archive)
		site := quickStart(t, bin, toolDir)
		if _, err := os.Stat(filepath.Join(site, "go.mod")); !os.IsNotExist(err) {
			t.Error("theme-copy site has a go.mod")
		}
		if got := readFile(t, filepath.Join(site, "themes", "contact-sheet", "VERSION")); strings.TrimSpace(got) != "v9.9.9" {
			t.Errorf("installed theme VERSION = %q", got)
		}
		build(t, site, nil)
		checkSite(t, site)
		// theme update replaces the copy.
		runCmd(t, repo, "bash", "scripts/package-theme.sh", "v9.9.10", archive)
		if out := runCmd(t, site, bin, "theme", "update"); !strings.Contains(out, "v9.9.9 -> v9.9.10") {
			t.Errorf("theme update output: %s", out)
		}
	})

	t.Run("Hugo Module (--module)", func(t *testing.T) {
		if _, err := exec.LookPath("go"); err != nil {
			t.Skip("go not installed (needed by Hugo Modules)")
		}
		site := quickStart(t, bin, toolDir, "--module")
		build(t, site, []string{"HUGO_MODULE_REPLACEMENTS=github.com/PrzemekMalak/contact-sheet -> " + repo})
		checkSite(t, site)
		cmd := exec.Command(bin, "theme", "update")
		cmd.Dir = site
		out, err := cmd.CombinedOutput()
		if err == nil || !strings.Contains(string(out), "hugo mod get -u") {
			t.Errorf("theme update on a module site: err=%v out=%s", err, out)
		}
	})
}

// quickStart runs photo init + photo add and returns the site directory.
func quickStart(t *testing.T, bin, toolDir string, initFlags ...string) string {
	t.Helper()
	parent := t.TempDir()
	args := append([]string{"init", "my-photos", "--title", "My Photos", "--url", "https://photos.example.org"}, initFlags...)
	runCmd(t, parent, bin, args...)
	site := filepath.Join(parent, "my-photos")
	copyFile(t, filepath.Join(toolDir, "internal/imaging/testdata/landscape.jpg"), filepath.Join(parent, "first-shot.jpg"))
	runCmd(t, site, bin, "add", filepath.Join(parent, "first-shot.jpg"), "-a", "trip", "--featured")
	return site
}

func build(t *testing.T, site string, env []string) {
	t.Helper()
	cmd := exec.Command("hugo", "--minify")
	cmd.Dir = site
	cmd.Env = append(os.Environ(), env...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("hugo failed: %v\n%s", err, out)
	}
}

func checkSite(t *testing.T, site string) {
	t.Helper()
	page := func(rel string) string { return readFile(t, filepath.Join(site, "public", rel)) }
	home := page("index.html")
	for _, want := range []string{"<title>My Photos</title>", "first-shot", "/js/lib/photoswipe/"} {
		if !strings.Contains(home, want) {
			t.Errorf("homepage missing %q", want)
		}
	}
	if !strings.Contains(page("albums/trip/index.html"), "first-shot") {
		t.Error("album page missing the added photo")
	}
	if !strings.Contains(page("p/first-shot/index.html"), `content="https://photos.example.org/p/first-shot/"`) {
		t.Error("photo page missing og:url with the configured base URL")
	}
}
