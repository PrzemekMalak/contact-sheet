package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PrzemekMalak/contact-sheet/tool/internal/app"
	"github.com/PrzemekMalak/contact-sheet/tool/internal/config"
	"github.com/PrzemekMalak/contact-sheet/tool/internal/store"
)

// Every command prints its help with -h, --help or -help, outside a site
// (no contact-sheet.toml), without touching the disk or the network.
func TestHelpOutsideSite(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, c := range commands {
		for _, flag := range []string{"-h", "--help", "-help"} {
			var buf bytes.Buffer
			helpOut = &buf
			code := run([]string{c, flag})
			helpOut = os.Stdout
			if code != 0 {
				t.Errorf("photo %s %s: exit %d", c, flag, code)
			}
			if !strings.Contains(buf.String(), "photo "+c) {
				t.Errorf("photo %s %s: help doesn't name the command: %q", c, flag, buf.String())
			}
		}
	}
}

// Help shows the flags, with their defaults where they have one.
func TestHelpListsFlags(t *testing.T) {
	t.Chdir(t.TempDir())
	cases := map[string][]string{
		"add":    {"-a", "--tags", "--featured", "--no-exif", "-r"},
		"init":   {"--title", "--url", "--author", "--amplify", "--module"},
		"backup": {"--jobs", "4"},
		"gui":    {"--port", "8765"},
		"cdk":    {"--infra", "diff", "deploy"},
	}
	for c, want := range cases {
		var buf bytes.Buffer
		helpOut = &buf
		run([]string{c, "-h"})
		helpOut = os.Stdout
		for _, w := range want {
			if !strings.Contains(buf.String(), w) {
				t.Errorf("photo %s -h lacks %q:\n%s", c, w, buf.String())
			}
		}
	}
}

// A -h after `photo cdk <subcommand>` belongs to CDK, so it isn't ours.
func TestCDKSubcommandHelpGoesToCDK(t *testing.T) {
	if !wantsHelp("cdk", []string{"-h"}) || wantsHelp("cdk", []string{"deploy", "-h"}) {
		t.Fatal("photo cdk -h is ours; photo cdk deploy -h is CDK's")
	}
}

// docs/cli.md documents every command, and the usage text lists every one.
func TestCLIDocsCoverEveryCommand(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "docs", "cli.md"))
	if err != nil {
		t.Fatal(err)
	}
	doc := string(b)
	for _, c := range commands {
		if !strings.Contains(doc, "\n### `photo "+c) {
			t.Errorf("docs/cli.md has no \"### `photo %s\" section", c)
		}
		if !strings.Contains(usage, "  "+c) {
			t.Errorf("usage text lacks %q", c)
		}
	}
}

// writeSite makes a local-mode site with two photos in the current directory.
func writeSite(t *testing.T) {
	t.Helper()
	toml := "[project]\nassets_dir = \"assets/gallery\"\nphotos_index = \"data/photos.json\"\nalbums_dir = \"content/albums\"\ntags_dir = \"content/tags\"\n[images]\nthumb_max_edge = 100\npreview_max_edge = 200\nwebp_quality = 80\n[s3]\nbucket = \"\"\n"
	for _, d := range []string{"assets/gallery", "content/albums", "content/tags", "data"} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	photos := `[{"id":"a","thumb":"gallery/a-thumb.webp","preview":"gallery/a-preview.webp","original":"gallery/a-preview.webp","albums":["trip"],"tags":[]},` +
		`{"id":"b","thumb":"gallery/b-thumb.webp","preview":"gallery/b-preview.webp","original":"gallery/b-preview.webp","albums":["trip"],"tags":[]}]`
	for name, body := range map[string]string{"contact-sheet.toml": toml, "data/photos.json": photos} {
		if err := os.WriteFile(name, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// photo rm deletes its own originals right away, but leaves deletes the GUI
// queued and hasn't published yet.
func TestRmLeavesOtherQueuedDeletes(t *testing.T) {
	t.Chdir(t.TempDir())
	writeSite(t)
	if err := app.Queue(".", "gui-photo.jpg"); err != nil {
		t.Fatal(err)
	}
	if code := run([]string{"rm", "a"}); code != 0 {
		t.Fatalf("photo rm: exit %d", code)
	}
	if code := run([]string{"rm-album", "trip", "--purge-orphans"}); code != 0 {
		t.Fatalf("photo rm-album: exit %d", code)
	}
	keys, err := app.QueuedKeys(".")
	if err != nil || len(keys) != 1 || keys[0] != "gui-photo.jpg" {
		t.Fatalf("queue = %v, %v; want only the GUI's gui-photo.jpg", keys, err)
	}
}

// A delete the GUI queues while photo rm runs isn't the CLI's to delete.
func TestDeleteNowIgnoresDeletesQueuedMeanwhile(t *testing.T) {
	t.Chdir(t.TempDir())
	writeSite(t)
	cfg, err := config.Load(".")
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	err = deleteNow(cfg, st, func() ([]string, error) {
		if err := app.Queue(".", "gui-race.jpg"); err != nil { // the GUI, at the same moment
			return nil, err
		}
		return app.Remove(cfg, st, []string{"a"})
	})
	if err != nil {
		t.Fatal(err)
	}
	if keys, _ := app.QueuedKeys("."); len(keys) != 1 || keys[0] != "gui-race.jpg" {
		t.Fatalf("queue = %v, want [gui-race.jpg]", keys)
	}
}
