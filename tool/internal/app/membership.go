package app

import (
	"fmt"

	"github.com/PrzemekMalak/contact-sheet/tool/internal/catalog"
	"github.com/PrzemekMalak/contact-sheet/tool/internal/config"
	"github.com/PrzemekMalak/contact-sheet/tool/internal/content"
)

// Link adds the photos to an album, creating its page if needed.
func Link(cfg *config.Config, slug string, ids []string) error {
	if err := checkAlbums([]string{slug}); err != nil {
		return err
	}
	photos, err := loadCatalog(cfg)
	if err != nil {
		return err
	}
	thumb := ""
	for _, id := range ids {
		rec, ok := catalog.Get(photos, id)
		if !ok {
			return fmt.Errorf("unknown photo %q", id)
		}
		catalog.AddAlbum(&rec, slug)
		photos = catalog.Upsert(photos, rec)
		if thumb == "" {
			thumb = rec.Thumb
		}
	}
	if err := content.EnsureAlbum(cfg, slug, "", thumb); err != nil {
		return err
	}
	return catalog.Save(cfg, photos)
}

// Unlink removes the photos from an album.
func Unlink(cfg *config.Config, slug string, ids []string) error {
	photos, err := loadCatalog(cfg)
	if err != nil {
		return err
	}
	for _, id := range ids {
		rec, ok := catalog.Get(photos, id)
		if !ok {
			return fmt.Errorf("unknown photo %q", id)
		}
		catalog.RemoveAlbum(&rec, slug)
		photos = catalog.Upsert(photos, rec)
	}
	return catalog.Save(cfg, photos)
}

// Retag replaces a photo's tags.
func Retag(cfg *config.Config, id string, tags []string) error {
	photos, err := loadCatalog(cfg)
	if err != nil {
		return err
	}
	rec, ok := catalog.Get(photos, id)
	if !ok {
		return fmt.Errorf("unknown photo %q", id)
	}
	if tags == nil {
		tags = []string{}
	}
	rec.Tags = tags
	photos = catalog.Upsert(photos, rec)
	if err := catalog.Save(cfg, photos); err != nil {
		return err
	}
	if err := content.RebuildTagStubs(cfg, photos); err != nil {
		return err
	}
	return content.RebuildGearStubs(cfg, photos)
}
