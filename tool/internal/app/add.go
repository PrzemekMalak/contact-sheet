package app

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/PrzemekMalak/contact-sheet/tool/internal/catalog"
	"github.com/PrzemekMalak/contact-sheet/tool/internal/config"
	"github.com/PrzemekMalak/contact-sheet/tool/internal/content"
	"github.com/PrzemekMalak/contact-sheet/tool/internal/imaging"
	"github.com/PrzemekMalak/contact-sheet/tool/internal/store"
	"github.com/PrzemekMalak/contact-sheet/tool/internal/xexif"
)

// Unassigned is a reserved album name.
const Unassigned = "unassigned"

// AddOpts controls Add.
type AddOpts struct {
	Tags      []string
	Albums    []string
	Featured  bool
	WithExif  bool
	Recursive bool
}

// validSlug rejects slugs that are empty, dot segments, or contain path
// separators (they would escape or clobber the albums directory).
func validSlug(s string) error {
	if s == "" || s == "." || s == ".." || strings.ContainsAny(s, `/\`+string(filepath.Separator)) {
		return fmt.Errorf("invalid album name %q", s)
	}
	return nil
}

// ValidAlbumSlug is the single album-name validator (CLI and server): a safe
// path segment that is not the reserved virtual album.
func ValidAlbumSlug(s string) error {
	if err := validSlug(s); err != nil {
		return err
	}
	if s == Unassigned {
		return fmt.Errorf("album name %q is reserved", Unassigned)
	}
	return nil
}

func checkAlbums(slugs []string) error {
	for _, s := range slugs {
		if err := ValidAlbumSlug(s); err != nil {
			return err
		}
	}
	return nil
}

// slugifyName turns a file stem into a photo id. It shares the slug rule
// with content.Slugify, adding a "photo" fallback for an otherwise empty id.
func slugifyName(stem string) string {
	s := content.Slugify(stem)
	if s == "" {
		return "photo"
	}
	return s
}

func isImage(p string) bool {
	switch strings.ToLower(filepath.Ext(p)) {
	case ".jpg", ".jpeg":
		return true
	}
	return false
}

func expand(paths []string, recursive bool) ([]string, error) {
	var out []string
	for _, p := range paths {
		fi, err := os.Stat(p)
		if err != nil {
			return nil, err
		}
		switch {
		case fi.IsDir() && recursive:
			err := filepath.WalkDir(p, func(f string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if !d.IsDir() && isImage(f) {
					out = append(out, f)
				}
				return nil
			})
			if err != nil {
				return nil, err
			}
		case fi.IsDir():
			return nil, fmt.Errorf("%s is a directory (use -r)", p)
		default:
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out, nil
}

func dedup(in []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// loadCatalog is catalog.Load that treats a missing index as empty.
func loadCatalog(cfg *config.Config) ([]catalog.Record, error) {
	if _, err := os.Stat(cfg.PhotosIndex); os.IsNotExist(err) {
		return nil, nil
	}
	return catalog.Load(cfg)
}

// recordPrefix is the asset dir as referenced from records ("gallery").
func recordPrefix(cfg *config.Config) string {
	return strings.TrimPrefix(filepath.ToSlash(cfg.AssetsDir), "assets/")
}

func derivPaths(cfg *config.Config, id string) (thumb, preview string) {
	return filepath.Join(cfg.AssetsDir, id+"-thumb.webp"), filepath.Join(cfg.AssetsDir, id+"-preview.webp")
}

// Add imports photos into the catalog. Already-present ids only gain albums.
func Add(cfg *config.Config, st store.Store, paths []string, opts AddOpts) ([]catalog.Record, error) {
	if err := checkAlbums(opts.Albums); err != nil {
		return nil, err
	}
	files, err := expand(paths, opts.Recursive)
	if err != nil {
		return nil, err
	}
	photos, err := loadCatalog(cfg)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(cfg.AssetsDir, 0o755); err != nil {
		return nil, err
	}
	albums := dedup(opts.Albums)
	prefix := recordPrefix(cfg)
	var done []catalog.Record
	for _, src := range files {
		base := filepath.Base(src)
		id := slugifyName(strings.TrimSuffix(base, filepath.Ext(base)))
		upName := id + strings.ToLower(filepath.Ext(base))

		if rec, ok := catalog.Get(photos, id); ok {
			for _, a := range albums {
				catalog.AddAlbum(&rec, a)
			}
			photos = catalog.Upsert(photos, rec)
			for _, a := range albums {
				if err := content.EnsureAlbum(cfg, a, "", rec.Thumb); err != nil {
					return nil, err
				}
			}
			done = append(done, rec)
			continue
		}

		thumbDisk, prevDisk := derivPaths(cfg, id)
		if _, _, err := imaging.MakeDerivative(src, thumbDisk, cfg.ThumbMax, cfg.Quality); err != nil {
			return nil, fmt.Errorf("%s: thumb: %w", base, err)
		}
		pw, ph, err := imaging.MakeDerivative(src, prevDisk, cfg.PreviewMax, cfg.Quality)
		if err != nil {
			return nil, fmt.Errorf("%s: preview: %w", base, err)
		}
		w, h, err := imaging.Dimensions(src)
		if err != nil {
			return nil, fmt.Errorf("%s: dimensions: %w", base, err)
		}
		// Dimensions are stored (unrotated); match the upright preview.
		if (w > h) != (pw > ph) {
			w, h = h, w
		}
		exif := catalog.Exif{}
		taken := ""
		if opts.WithExif {
			if m, err := xexif.Extract(src); err == nil && m != nil {
				exif = catalog.Exif(m)
			}
			taken, _ = xexif.Taken(src)
		}
		url, err := st.Upload(upName, src)
		if err != nil {
			return nil, fmt.Errorf("%s: upload: %w", base, err)
		}
		if cfg.Bucket == "" {
			// Local mode: no upload happened; point at the local preview so
			// Hugo can resolve it.
			url = prefix + "/" + id + "-preview.webp"
		}
		tags := opts.Tags
		if tags == nil {
			tags = []string{}
		}
		rec := catalog.Record{
			ID: id, Thumb: prefix + "/" + id + "-thumb.webp", Preview: prefix + "/" + id + "-preview.webp",
			Original: url, Width: w, Height: h, PW: pw, PH: ph,
			Tags: tags, Albums: albums, Featured: opts.Featured, Exif: exif, Taken: taken,
		}
		photos = catalog.Upsert(photos, rec)
		for _, a := range albums {
			if err := content.EnsureAlbum(cfg, a, "", rec.Thumb); err != nil {
				return nil, err
			}
		}
		done = append(done, rec)
	}
	if err := catalog.Save(cfg, photos); err != nil {
		return nil, err
	}
	if err := content.RebuildTagStubs(cfg, photos); err != nil {
		return nil, err
	}
	if err := content.RebuildGearStubs(cfg, photos); err != nil {
		return nil, err
	}
	return done, nil
}
