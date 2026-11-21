// Package scaffold creates a new site that uses the contact-sheet theme.
package scaffold

import (
	"bytes"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/PrzemekMalak/contact-sheet/tool/internal/content"
)

// ThemeModule is the Hugo Module path new sites import.
const ThemeModule = "github.com/PrzemekMalak/contact-sheet"

//go:embed all:files
var files embed.FS

// Options are the site settings written into hugo.toml.
type Options struct {
	Title   string // default: the directory name
	BaseURL string // default: https://example.com/
	Author  string // default: the title
	Amplify bool   // also write amplify.yml for AWS Amplify hosting
	// Module imports the theme as a Hugo Module (needs Go) instead of naming
	// a copy in themes/ that the caller installs.
	Module bool
}

// Init writes a new site into dir, which must be missing or empty. It
// returns the created files, relative to dir.
func Init(dir string, o Options) ([]string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(abs)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if len(entries) > 0 {
		return nil, fmt.Errorf("%s is not empty; photo init needs a new or empty directory", dir)
	}
	name := filepath.Base(abs)
	if o.Title == "" {
		o.Title = name
	}
	if o.Author == "" {
		o.Author = o.Title
	}
	if o.BaseURL == "" {
		o.BaseURL = "https://example.com/"
	}
	if !strings.HasSuffix(o.BaseURL, "/") {
		o.BaseURL += "/"
	}
	module := content.Slugify(name)
	if module == "" {
		module = "site"
	}

	hugo, err := render("files/hugo.toml.tmpl", map[string]any{
		"Title": o.Title, "BaseURL": o.BaseURL, "Author": o.Author, "ThemeModule": ThemeModule,
		"Module": o.Module,
	})
	if err != nil {
		return nil, err
	}
	type file struct {
		path string
		data []byte
	}
	out := []file{
		{"hugo.toml", hugo},
		{"contact-sheet.toml", mustRead("files/contact-sheet.toml")},
		{"content/_index.md", mustRead("files/content/_index.md")},
		{"content/about.md", mustRead("files/content/about.md")},
		{".gitignore", mustRead("files/gitignore")},
	}
	if o.Module {
		out = append(out, file{"go.mod", []byte("module " + module + "\n\ngo 1.22\n")})
	}
	if o.Amplify {
		amplify := "files/amplify.yml"
		if o.Module {
			amplify = "files/amplify-module.yml"
		}
		out = append(out, file{"amplify.yml", mustRead(amplify)})
	}
	var created []string
	for _, f := range out {
		p := filepath.Join(abs, filepath.FromSlash(f.path))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return created, err
		}
		if err := os.WriteFile(p, f.data, 0o644); err != nil {
			return created, err
		}
		created = append(created, f.path)
	}
	return created, nil
}

func mustRead(name string) []byte {
	b, err := files.ReadFile(name)
	if err != nil {
		panic(err) // embedded at build time; missing means a broken build
	}
	return b
}

func render(name string, data any) ([]byte, error) {
	t, err := template.New(filepath.Base(name)).Funcs(template.FuncMap{"toml": tomlString}).ParseFS(files, name)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// tomlString quotes s as a TOML basic string.
func tomlString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"':
			b.WriteString(`\"`)
		case r == '\\':
			b.WriteString(`\\`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\t':
			b.WriteString(`\t`)
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&b, `\u%04X`, r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}
