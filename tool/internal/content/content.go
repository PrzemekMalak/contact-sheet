// Package content writes Hugo album pages and tag stub files.
package content

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/PrzemekMalak/contact-sheet/tool/internal/catalog"
	"github.com/PrzemekMalak/contact-sheet/tool/internal/config"
)

func yamlDQ(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), `"`, `\"`)
}

var nonSlug = regexp.MustCompile("[^a-z0-9]+")

// Slugify lowercases, replaces runs of non-alphanumeric characters with "-",
// and trims leading/trailing "-". Returns "" when nothing alphanumeric remains.
// This rule is shared with the Hugo templates (replaceRE "[^a-z0-9]+" "-").
func Slugify(s string) string {
	return strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(s), "-"), "-")
}

func titleize(slug string) string {
	words := strings.Fields(strings.ReplaceAll(slug, "-", " "))
	for i, w := range words {
		words[i] = strings.ToUpper(w[:1]) + strings.ToLower(w[1:])
	}
	return strings.Join(words, " ")
}

func albumPath(cfg *config.Config, slug string) string {
	return filepath.Join(cfg.AlbumsDir, slug, "index.md")
}

// EnsureAlbum creates the album page if absent; an existing page is left untouched.
func EnsureAlbum(cfg *config.Config, slug, title, coverThumb string) error {
	p := albumPath(cfg, slug)
	if _, err := os.Stat(p); err == nil {
		return nil
	}
	if title == "" {
		title = titleize(slug)
	}
	lines := []string{
		"---",
		fmt.Sprintf("title: \"%s\"", yamlDQ(title)),
		"date: " + time.Now().Format("2006-01-02"),
		fmt.Sprintf("gallery: \"%s\"", yamlDQ(slug)),
	}
	if coverThumb != "" {
		lines = append(lines, fmt.Sprintf("cover: \"%s\"", yamlDQ(coverThumb)))
	}
	lines = append(lines, "---", "", "")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, []byte(strings.Join(lines, "\n")), 0o644)
}

// SetCover sets the cover: front-matter line of an existing album page.
func SetCover(cfg *config.Config, slug, coverThumb string) error {
	return setFrontMatter(albumPath(cfg, slug), "cover", coverThumb)
}

// SetTitle sets the title: of an album page, creating the page if needed.
// The slug (the album's URL) doesn't change.
func SetTitle(cfg *config.Config, slug, title string) error {
	p := albumPath(cfg, slug)
	if _, err := os.Stat(p); os.IsNotExist(err) {
		return EnsureAlbum(cfg, slug, title, "")
	}
	return setFrontMatter(p, "title", title)
}

// AlbumTitle reads title: from an album page's front matter. It returns ""
// when the page or the key is missing.
func AlbumTitle(cfg *config.Config, slug string) (string, error) {
	b, err := os.ReadFile(albumPath(cfg, slug))
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(b), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return "", nil
	}
	for _, l := range lines[1:] {
		if strings.TrimSpace(l) == "---" {
			break
		}
		if v, ok := strings.CutPrefix(l, "title:"); ok {
			return yamlScalar(strings.TrimSpace(v)), nil
		}
	}
	return "", nil
}

// yamlScalar decodes the simple YAML scalars album pages use: "double
// quoted" (with \" and \\ escapes, as yamlDQ writes them), 'single quoted'
// (two single quotes for one), or plain.
func yamlScalar(v string) string {
	switch {
	case len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"':
		var b strings.Builder
		in := v[1 : len(v)-1]
		for i := 0; i < len(in); i++ {
			if in[i] == '\\' && i+1 < len(in) {
				i++
			}
			b.WriteByte(in[i])
		}
		return b.String()
	case len(v) >= 2 && v[0] == '\'' && v[len(v)-1] == '\'':
		return strings.ReplaceAll(v[1:len(v)-1], "''", "'")
	}
	return v
}

// setFrontMatter sets key: "value" in the front matter of the page at p,
// replacing an existing line or adding one before the closing ---.
func setFrontMatter(p, key, value string) error {
	b, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	line := fmt.Sprintf("%s: \"%s\"", key, yamlDQ(value))
	// Keep the file's line endings (CRLF on Windows checkouts).
	nl := "\n"
	if strings.Contains(string(b), "\r\n") {
		nl = "\r\n"
	}
	lines := strings.Split(string(b), nl)
	if lines[0] != "---" {
		return fmt.Errorf("%s: no front matter", p)
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if lines[i] == "---" {
			end = i
			break
		}
	}
	if end < 0 {
		return fmt.Errorf("%s: unterminated front matter", p)
	}
	for i := 1; i < end; i++ {
		if strings.HasPrefix(lines[i], key+":") {
			lines[i] = line
			return os.WriteFile(p, []byte(strings.Join(lines, nl)), 0o644)
		}
	}
	out := append([]string{}, lines[:end]...)
	out = append(out, line)
	out = append(out, lines[end:]...)
	return os.WriteFile(p, []byte(strings.Join(out, nl)), 0o644)
}

// RebuildTagStubs creates missing tag stubs and prunes stale ones (never _index.md).
func RebuildTagStubs(cfg *config.Config, photos []catalog.Record) error {
	set := map[string]bool{}
	for _, r := range photos {
		for _, t := range r.Tags {
			set[t] = true
		}
	}
	tags := make([]string, 0, len(set))
	for t := range set {
		tags = append(tags, t)
	}
	sort.Strings(tags)
	if err := os.MkdirAll(cfg.TagsDir, 0o755); err != nil {
		return err
	}
	wanted := map[string]bool{}
	for _, t := range tags {
		name := t + ".md"
		wanted[name] = true
		p := filepath.Join(cfg.TagsDir, name)
		if _, err := os.Stat(p); err == nil {
			continue
		}
		body := fmt.Sprintf("---\ntitle: \"%s\"\ntag: \"%s\"\n---\n", yamlDQ(t), yamlDQ(t))
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			return err
		}
	}
	matches, err := filepath.Glob(filepath.Join(cfg.TagsDir, "*.md"))
	if err != nil {
		return err
	}
	for _, m := range matches {
		base := filepath.Base(m)
		if base == "_index.md" || wanted[base] {
			continue
		}
		if err := os.Remove(m); err != nil {
			return err
		}
	}
	return nil
}

func gearDir(cfg *config.Config) string {
	return filepath.Join(filepath.Dir(cfg.AlbumsDir), "gear")
}

type gearInfo struct {
	value    string // exact EXIF string
	gearType string // "camera", "lens" or "combo"
	camera   string // combo only
	lens     string // combo only
}

// gearStub is the camera/lens page the tool writes, with the given title.
func gearStub(title, slug string, g gearInfo) string {
	return fmt.Sprintf("---\ntitle: \"%s\"\nslug: \"%s\"\ngear: \"%s\"\ngearType: \"%s\"\n---\n",
		yamlDQ(title), slug, yamlDQ(g.value), g.gearType)
}

// RebuildGearStubs writes a content/gear/<slug>/index.md page bundle for each
// distinct non-empty exif.camera and exif.lens across photos (skipping values
// whose slug is empty), plus one "combo" page per co-occurring camera+lens
// pair (slug = Slugify(camera + " " + lens)), and prunes any content/gear/<slug>/ dir not in the
// current set. A content/gear/_index.md file is preserved.
func RebuildGearStubs(cfg *config.Config, photos []catalog.Record) error {
	dir := gearDir(cfg)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	// Collect distinct slugs. First writer wins on a slug collision.
	want := map[string]gearInfo{}
	add := func(value, gearType string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		slug := Slugify(value)
		if slug == "" {
			return
		}
		if _, ok := want[slug]; !ok {
			want[slug] = gearInfo{value: value, gearType: gearType}
		}
	}
	for _, r := range photos {
		cam, _ := r.Exif["camera"].(string)
		lens, _ := r.Exif["lens"].(string)
		cam, lens = strings.TrimSpace(cam), strings.TrimSpace(lens)
		add(cam, "camera")
		add(lens, "lens")
		if cam != "" && lens != "" {
			if slug := Slugify(cam + " " + lens); slug != "" {
				if _, ok := want[slug]; !ok {
					want[slug] = gearInfo{gearType: "combo", camera: cam, lens: lens}
				}
			}
		}
	}
	slugs := make([]string, 0, len(want))
	for s := range want {
		slugs = append(slugs, s)
	}
	sort.Strings(slugs)
	for _, slug := range slugs {
		g := want[slug]
		p := filepath.Join(dir, slug, "index.md")
		var body string
		if g.gearType == "combo" {
			body = fmt.Sprintf("---\ntitle: \"%s\"\nslug: \"%s\"\ngearType: \"combo\"\ncamera: \"%s\"\nlens: \"%s\"\n---\n",
				yamlDQ(g.camera+" + "+g.lens), slug, yamlDQ(g.camera), yamlDQ(g.lens))
		} else {
			body = gearStub(GearTitle(g.value, g.gearType), slug, g)
		}
		if old, err := os.ReadFile(p); err == nil {
			// Never overwrite a page the owner edited. A camera/lens page still
			// exactly as an older tool wrote it (title == raw EXIF value) gets
			// the cleaned title once.
			if g.gearType == "combo" || string(old) != gearStub(g.value, slug, g) || string(old) == body {
				continue
			}
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			return err
		}
	}
	// Prune stale gear dirs (keep _index.md and any wanted slug dir).
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue // keep _index.md or other files
		}
		if _, ok := want[e.Name()]; ok {
			continue
		}
		if err := os.RemoveAll(filepath.Join(dir, e.Name())); err != nil {
			return err
		}
	}
	return nil
}
