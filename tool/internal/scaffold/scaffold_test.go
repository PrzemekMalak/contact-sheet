package scaffold_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"

	"github.com/PrzemekMalak/contact-sheet/tool/internal/cdk"
	"github.com/PrzemekMalak/contact-sheet/tool/internal/config"
	"github.com/PrzemekMalak/contact-sheet/tool/internal/scaffold"
)

type hugoConfig struct {
	BaseURL string `toml:"baseURL"`
	Title   string `toml:"title"`
	Theme   string `toml:"theme"`
	Params  struct {
		Author   string `toml:"author"`
		HomeMode string `toml:"homeMode"`
	} `toml:"params"`
	Module struct {
		Imports []struct {
			Path string `toml:"path"`
		} `toml:"imports"`
	} `toml:"module"`
}

func readHugo(t *testing.T, dir string) hugoConfig {
	t.Helper()
	var h hugoConfig
	if _, err := toml.DecodeFile(filepath.Join(dir, "hugo.toml"), &h); err != nil {
		t.Fatalf("hugo.toml is not valid TOML: %v", err)
	}
	return h
}

func TestInitCreatesSite(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "My Photos")
	files, err := scaffold.Init(dir, scaffold.Options{
		Title: `Jane's "Photos" \ more`, BaseURL: "https://photos.example.org", Author: "Jane Doe",
		Module: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"hugo.toml", "contact-sheet.toml", "go.mod", "content/_index.md", "content/about.md", ".gitignore"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("missing %s: %v", f, err)
		}
	}
	if len(files) < 6 {
		t.Errorf("Init reported %d files: %v", len(files), files)
	}
	h := readHugo(t, dir)
	if h.Title != `Jane's "Photos" \ more` {
		t.Errorf("title = %q", h.Title)
	}
	if h.BaseURL != "https://photos.example.org/" {
		t.Errorf("baseURL = %q, want trailing slash", h.BaseURL)
	}
	if h.Params.Author != "Jane Doe" {
		t.Errorf("author = %q", h.Params.Author)
	}
	if len(h.Module.Imports) != 1 || h.Module.Imports[0].Path != scaffold.ThemeModule || h.Theme != "" {
		t.Errorf("module mode: imports = %+v, theme = %q", h.Module.Imports, h.Theme)
	}
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("contact-sheet.toml does not load: %v", err)
	}
	if cfg.Bucket != "" || cfg.AssetsDir != "assets/gallery" || cfg.SiteTitle != h.Title {
		t.Errorf("config = %+v", cfg)
	}
	gomod, _ := os.ReadFile(filepath.Join(dir, "go.mod"))
	if !strings.HasPrefix(string(gomod), "module my-photos\n") {
		t.Errorf("go.mod = %q", gomod)
	}
	if _, err := os.Stat(filepath.Join(dir, "amplify.yml")); !os.IsNotExist(err) {
		t.Error("amplify.yml written without Amplify option")
	}
}

// The default needs no Go: hugo.toml names the theme copied into themes/.
func TestInitThemeCopyMode(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "site")
	files, err := scaffold.Init(dir, scaffold.Options{})
	if err != nil {
		t.Fatal(err)
	}
	h := readHugo(t, dir)
	if h.Theme != "contact-sheet" || len(h.Module.Imports) != 0 {
		t.Errorf("copy mode: theme = %q, imports = %+v", h.Theme, h.Module.Imports)
	}
	if _, err := os.Stat(filepath.Join(dir, "go.mod")); !os.IsNotExist(err) {
		t.Error("go.mod written in theme-copy mode")
	}
	for _, f := range files {
		if f == "go.mod" {
			t.Error("go.mod reported as created")
		}
	}
}

func TestInitDefaults(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "holiday-pics")
	if _, err := scaffold.Init(dir, scaffold.Options{}); err != nil {
		t.Fatal(err)
	}
	h := readHugo(t, dir)
	if h.Title != "holiday-pics" || h.Params.Author != "holiday-pics" || h.BaseURL != "https://example.com/" {
		t.Errorf("defaults: %+v", h)
	}
}

func TestInitAmplify(t *testing.T) {
	for _, module := range []bool{false, true} {
		dir := t.TempDir()
		if _, err := scaffold.Init(dir, scaffold.Options{Amplify: true, Module: module}); err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(filepath.Join(dir, "amplify.yml"))
		if err != nil {
			t.Fatal(err)
		}
		installsGo := strings.Contains(string(b), "go${GO_VERSION}.linux-amd64.tar.gz")
		if installsGo != module {
			t.Errorf("module=%v: amplify.yml installs Go = %v", module, installsGo)
		}
	}
}

func TestInitRefusesNonEmptyDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "keep.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := scaffold.Init(dir, scaffold.Options{}); err == nil {
		t.Fatal("Init into a non-empty directory succeeded")
	}
	if _, err := os.Stat(filepath.Join(dir, "hugo.toml")); !os.IsNotExist(err) {
		t.Error("Init wrote files into a non-empty directory")
	}
}

func TestInitCDKSectionCommented(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "site")
	if _, err := scaffold.Init(dir, scaffold.Options{}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "contact-sheet.toml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range []string{"# [cdk]", "# [cdk.context]"} {
		if !strings.Contains(string(b), w) {
			t.Errorf("contact-sheet.toml lacks %q", w)
		}
	}
	s, err := cdk.LoadSettings(dir)
	if err != nil {
		t.Fatal(err)
	}
	if s.Infra != "" || s.Profile != "" || len(s.Context) != 0 {
		t.Fatalf("commented section must load as empty, got %+v", s)
	}
}
