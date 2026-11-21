// Package theme installs the contact-sheet Hugo theme into a site's
// themes/ directory from a release archive, so a site needs no Go toolchain
// (unlike a Hugo Module).
package theme

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Name is the theme's directory under themes/ and the value of `theme` in hugo.toml.
const Name = "contact-sheet"

const (
	releases  = "https://github.com/PrzemekMalak/contact-sheet/releases"
	assetName = "contact-sheet-theme.tar.gz"
	// EnvSource overrides where the theme archive comes from (URL or local path).
	EnvSource = "CONTACT_SHEET_THEME"
)

var releaseTag = regexp.MustCompile(`^v\d+\.\d+\.\d+$`)

// Source is the archive location for a tool version: the matching release
// for a tagged build (vX.Y.Z), otherwise the latest release.
func Source(version string) string {
	if s := os.Getenv(EnvSource); s != "" {
		return s
	}
	if releaseTag.MatchString(version) {
		return releases + "/download/" + version + "/" + assetName
	}
	return releases + "/latest/download/" + assetName
}

// Dir is the theme's directory in a site.
func Dir(siteDir string) string { return filepath.Join(siteDir, "themes", Name) }

// InstalledVersion reads the VERSION file of the theme copy in siteDir.
func InstalledVersion(siteDir string) (string, error) {
	b, err := os.ReadFile(filepath.Join(Dir(siteDir), "VERSION"))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

// Install downloads the archive at src (an http(s) URL or a local path) and
// installs it as siteDir/themes/contact-sheet, replacing any existing copy
// only once the new one is fully extracted. It returns the new version.
func Install(siteDir, src string) (string, error) {
	rc, err := open(src)
	if err != nil {
		return "", err
	}
	defer rc.Close()

	themes := filepath.Join(siteDir, "themes")
	if err := os.MkdirAll(themes, 0o755); err != nil {
		return "", err
	}
	tmp, err := os.MkdirTemp(themes, ".contact-sheet-new-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	if err := extract(rc, tmp); err != nil {
		return "", fmt.Errorf("theme archive %s: %w", src, err)
	}
	b, err := os.ReadFile(filepath.Join(tmp, "VERSION"))
	if err != nil {
		return "", fmt.Errorf("theme archive %s has no VERSION file", src)
	}

	dst := Dir(siteDir)
	old := dst + ".old"
	os.RemoveAll(old)
	if _, err := os.Stat(dst); err == nil {
		if err := os.Rename(dst, old); err != nil {
			return "", err
		}
	}
	if err := os.Rename(tmp, dst); err != nil {
		os.Rename(old, dst) // put the previous version back
		return "", err
	}
	os.RemoveAll(old)
	return strings.TrimSpace(string(b)), nil
}

func open(src string) (io.ReadCloser, error) {
	if !strings.HasPrefix(src, "http://") && !strings.HasPrefix(src, "https://") {
		return os.Open(src)
	}
	client := &http.Client{Timeout: 2 * time.Minute}
	resp, err := client.Get(src)
	if err != nil {
		return nil, fmt.Errorf("download theme: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("download theme %s: HTTP %d", src, resp.StatusCode)
	}
	return resp.Body, nil
}

// extract unpacks a .tar.gz into dir, accepting only regular files and
// directories that stay inside dir.
func extract(r io.Reader, dir string) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		name := path.Clean(strings.TrimPrefix(h.Name, "./"))
		if name == "." {
			continue
		}
		if path.IsAbs(h.Name) || name == ".." || strings.HasPrefix(name, "../") {
			return fmt.Errorf("unsafe path %q", h.Name)
		}
		target := filepath.Join(dir, filepath.FromSlash(name))
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, io.LimitReader(tr, 64<<20)); err != nil {
				f.Close()
				return err
			}
			if err := f.Close(); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported entry %q (type %c)", h.Name, h.Typeflag)
		}
	}
}
