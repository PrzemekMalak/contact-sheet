package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PrzemekMalak/contact-sheet/tool/internal/config"
)

const fixture = `[project]
assets_dir = "assets/gallery"
photos_index = "data/photos.json"
albums_dir = "content/albums"
tags_dir = "content/tags"

[images]
thumb_max_edge = 400
preview_max_edge = 1600
webp_quality = 82

[s3]
bucket =  "photositestack-originalsbucket-example"
originals_prefix = "originals"
cdn_base = "https://cdn.example.com"
region = "us-east-1"   # matches the CDK stack region
`

func TestLoadParsesSections(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "contact-sheet.toml"), []byte(fixture), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	strs := []struct{ name, got, want string }{
		{"AssetsDir", c.AssetsDir, "assets/gallery"},
		{"PhotosIndex", c.PhotosIndex, "data/photos.json"},
		{"AlbumsDir", c.AlbumsDir, "content/albums"},
		{"TagsDir", c.TagsDir, "content/tags"},
		{"Bucket", c.Bucket, "photositestack-originalsbucket-example"},
		{"OriginalsPrefix", c.OriginalsPrefix, "originals"},
		{"CDNBase", c.CDNBase, "https://cdn.example.com"},
		{"Region", c.Region, "us-east-1"},
	}
	for _, s := range strs {
		if s.got != s.want {
			t.Errorf("%s = %q, want %q", s.name, s.got, s.want)
		}
	}
	ints := []struct {
		name      string
		got, want int
	}{
		{"ThumbMax", c.ThumbMax, 400},
		{"PreviewMax", c.PreviewMax, 1600},
		{"Quality", c.Quality, 82},
	}
	for _, i := range ints {
		if i.got != i.want {
			t.Errorf("%s = %d, want %d", i.name, i.got, i.want)
		}
	}
}

func TestLoadProfile(t *testing.T) {
	dir := t.TempDir()
	toml := "[s3]\nbucket = \"b\"\nregion = \"us-east-1\"\nprofile = \"gallery\"\n"
	if err := os.WriteFile(filepath.Join(dir, "contact-sheet.toml"), []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if c.Profile != "gallery" {
		t.Fatalf("Profile = %q, want %q", c.Profile, "gallery")
	}
}

func TestLoadProfileDefaultsEmpty(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "contact-sheet.toml"), []byte(fixture), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if c.Profile != "" {
		t.Fatalf("Profile = %q, want empty when no profile key", c.Profile)
	}
}

func TestLoadSiteTitleFromHugo(t *testing.T) {
	dir := t.TempDir()
	must := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	must("contact-sheet.toml", fixture)
	must("hugo.toml", "baseURL = \"https://example.com/\"\ntitle = \"example.com\"\n[params]\n  author = \"A\"\n")
	c, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if c.SiteTitle != "example.com" {
		t.Fatalf("SiteTitle = %q, want %q", c.SiteTitle, "example.com")
	}
}

func TestLoadSiteTitleMissingHugo(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "contact-sheet.toml"), []byte(fixture), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if c.SiteTitle != "" {
		t.Fatalf("SiteTitle = %q, want empty without hugo.toml", c.SiteTitle)
	}
}

func TestLoadPrefersContactSheetToml(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "contact-sheet.toml"), []byte("[s3]\nbucket = \"new\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mygallery.toml"), []byte("[s3]\nbucket = \"old\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if c.Bucket != "new" {
		t.Fatalf("Bucket = %q, want %q from contact-sheet.toml", c.Bucket, "new")
	}
}

func TestLoadMissingConfigNamesNewFile(t *testing.T) {
	_, err := config.Load(t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "contact-sheet.toml") {
		t.Fatalf("err = %v, want it to mention contact-sheet.toml", err)
	}
}

func TestLoadFallsBackToLegacyMygalleryToml(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "mygallery.toml"), []byte(fixture), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if c.AssetsDir != "assets/gallery" {
		t.Fatalf("AssetsDir = %q, want it read from mygallery.toml", c.AssetsDir)
	}
}
