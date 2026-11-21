// Package config loads the photo catalog settings from contact-sheet.toml
// (or the legacy mygallery.toml), plus the site title from Hugo's hugo.toml
// so the site name lives in one place.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Config holds the settings the photo tool needs.
type Config struct {
	// SiteTitle is hugo.toml's top-level title; empty when hugo.toml is absent.
	SiteTitle string

	Bucket          string
	OriginalsPrefix string
	CDNBase         string
	Region          string
	// Profile is the AWS shared-config profile to use for S3 credentials.
	// Empty means the default credential chain (env vars, default profile).
	Profile string

	AssetsDir   string
	PhotosIndex string
	AlbumsDir   string
	TagsDir     string

	ThumbMax   int
	PreviewMax int
	Quality    int
}

type file struct {
	Project struct {
		AssetsDir   string `toml:"assets_dir"`
		PhotosIndex string `toml:"photos_index"`
		AlbumsDir   string `toml:"albums_dir"`
		TagsDir     string `toml:"tags_dir"`
	} `toml:"project"`
	Images struct {
		ThumbMax   int `toml:"thumb_max_edge"`
		PreviewMax int `toml:"preview_max_edge"`
		Quality    int `toml:"webp_quality"`
	} `toml:"images"`
	S3 struct {
		Bucket          string `toml:"bucket"`
		OriginalsPrefix string `toml:"originals_prefix"`
		CDNBase         string `toml:"cdn_base"`
		Region          string `toml:"region"`
		Profile         string `toml:"profile"`
	} `toml:"s3"`
}

// FileName is the tool's config file; LegacyFileName is read when it is absent.
const (
	FileName       = "contact-sheet.toml"
	LegacyFileName = "mygallery.toml"
)

// Load reads <dir>/contact-sheet.toml (falling back to <dir>/mygallery.toml)
// and the title from <dir>/hugo.toml.
func Load(dir string) (*Config, error) {
	var f file
	path := filepath.Join(dir, FileName)
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		legacy := filepath.Join(dir, LegacyFileName)
		if _, err := os.Stat(legacy); err != nil {
			return nil, fmt.Errorf("no %s in %s (run photo from the site root)", FileName, dir)
		}
		path = legacy
	}
	if _, err := toml.DecodeFile(path, &f); err != nil {
		return nil, fmt.Errorf("load config %s: %w", path, err)
	}
	var hugo struct {
		Title string `toml:"title"`
	}
	hugoPath := filepath.Join(dir, "hugo.toml")
	if _, err := toml.DecodeFile(hugoPath, &hugo); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("load config %s: %w", hugoPath, err)
	}
	return &Config{
		SiteTitle:       hugo.Title,
		Bucket:          f.S3.Bucket,
		OriginalsPrefix: f.S3.OriginalsPrefix,
		CDNBase:         f.S3.CDNBase,
		Region:          f.S3.Region,
		Profile:         f.S3.Profile,
		AssetsDir:       f.Project.AssetsDir,
		PhotosIndex:     f.Project.PhotosIndex,
		AlbumsDir:       f.Project.AlbumsDir,
		TagsDir:         f.Project.TagsDir,
		ThumbMax:        f.Images.ThumbMax,
		PreviewMax:      f.Images.PreviewMax,
		Quality:         f.Images.Quality,
	}, nil
}
