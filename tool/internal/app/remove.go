package app

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/PrzemekMalak/contact-sheet/tool/internal/catalog"
	"github.com/PrzemekMalak/contact-sheet/tool/internal/config"
	"github.com/PrzemekMalak/contact-sheet/tool/internal/content"
	"github.com/PrzemekMalak/contact-sheet/tool/internal/store"
)

// purge deletes derivatives and queues the original for deferred S3 deletion,
// returning the queued key. Call only after the catalog no longer lists the
// photo and has been saved.
func purge(cfg *config.Config, rec catalog.Record) (string, error) {
	t, p := derivPaths(cfg, rec.ID)
	for _, f := range []string{t, p} {
		if err := os.Remove(f); err != nil && !os.IsNotExist(err) {
			return "", err
		}
	}
	key := objectKey(cfg, rec.Original)
	return key, Queue(".", key)
}

// Remove deletes photos and returns the S3 keys it queued; the S3 deletes
// themselves are not performed (see DeleteDue and DeleteNow).
// Order: validate all ids, save the catalog, and only then delete derivatives
// and queue S3 deletes, so a failure never orphans a still-listed photo.
func Remove(cfg *config.Config, _ store.Store, ids []string) (queued []string, err error) {
	photos, err := loadCatalog(cfg)
	if err != nil {
		return nil, err
	}
	var gone []catalog.Record
	for _, id := range ids {
		rec, ok := catalog.Get(photos, id)
		if !ok {
			return nil, fmt.Errorf("unknown photo %q", id)
		}
		gone = append(gone, rec)
	}
	for _, rec := range gone {
		photos = catalog.Remove(photos, rec.ID)
	}
	if err := catalog.Save(cfg, photos); err != nil {
		return nil, err
	}
	for _, rec := range gone {
		key, err := purge(cfg, rec)
		if key != "" {
			queued = append(queued, key)
		}
		if err != nil {
			return queued, err
		}
	}
	if err := content.RebuildTagStubs(cfg, photos); err != nil {
		return queued, err
	}
	return queued, content.RebuildGearStubs(cfg, photos)
}

// RemoveAlbum unlinks every photo from the album and deletes its page.
// It returns the S3 keys of purged photos it queued.
func RemoveAlbum(cfg *config.Config, _ store.Store, slug string, purgeOrphans bool) (unlinked, orphans, purged int, queued []string, err error) {
	if err = checkAlbums([]string{slug}); err != nil {
		return
	}
	photos, err := loadCatalog(cfg)
	if err != nil {
		return
	}
	keep := []catalog.Record{}
	var gone []catalog.Record
	for _, rec := range photos {
		if !slices.Contains(rec.Albums, slug) {
			keep = append(keep, rec)
			continue
		}
		catalog.RemoveAlbum(&rec, slug)
		unlinked++
		if len(rec.Albums) == 0 {
			orphans++
			if purgeOrphans {
				purged++
				gone = append(gone, rec)
				continue
			}
		}
		keep = append(keep, rec)
	}
	if err = catalog.Save(cfg, keep); err != nil {
		return
	}
	for _, rec := range gone {
		var key string
		key, err = purge(cfg, rec)
		if key != "" {
			queued = append(queued, key)
		}
		if err != nil {
			return
		}
	}
	if err = os.RemoveAll(filepath.Join(cfg.AlbumsDir, slug)); err != nil {
		return
	}
	if err = content.RebuildTagStubs(cfg, keep); err != nil {
		return
	}
	err = content.RebuildGearStubs(cfg, keep)
	return
}

// Sync re-saves the catalog (sorted) and rebuilds tag stubs.
func Sync(cfg *config.Config) error {
	photos, err := loadCatalog(cfg)
	if err != nil {
		return err
	}
	if err := catalog.Save(cfg, photos); err != nil {
		return err
	}
	if err := content.RebuildTagStubs(cfg, photos); err != nil {
		return err
	}
	return content.RebuildGearStubs(cfg, photos)
}
