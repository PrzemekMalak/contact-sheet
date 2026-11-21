package app

import (
	"errors"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/PrzemekMalak/contact-sheet/tool/internal/catalog"
	"github.com/PrzemekMalak/contact-sheet/tool/internal/store"
)

// delSpy records S3 deletes; fail makes Delete fail for that key.
type delSpy struct {
	deleted []string
	fail    string
}

func (d *delSpy) Upload(f, p string) (string, error) { return "", nil }
func (d *delSpy) Delete(f string) error {
	if f == d.fail {
		return errors.New("boom")
	}
	d.deleted = append(d.deleted, f)
	return nil
}
func (d *delSpy) Copy(a, b string) error { return nil }
func (d *delSpy) Key(f string) string    { return f }

var _ store.Store = (*delSpy)(nil)

var t0 = time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)

func TestQueueDedup(t *testing.T) {
	newRepo(t)
	must(t, Queue(".", "a.jpg"))
	must(t, Queue(".", "a.jpg"))
	if n, _ := Count("."); n != 1 {
		t.Fatalf("dedup failed: %d", n)
	}
}

func TestDeleteDueWaitsForPublishAndGrace(t *testing.T) {
	cfg, _ := newRepo(t)
	sp := &delSpy{}
	must(t, Queue(".", "a.jpg"))
	// Not published yet: nothing is due, however late it is.
	if n, err := DeleteDue(".", cfg, sp, t0.Add(30*24*time.Hour)); err != nil || n != 0 {
		t.Fatalf("unpublished: deleted %d, %v", n, err)
	}
	publishAll(t, t0)
	if n, _ := Count("."); n != 0 {
		t.Errorf("published entries must not count as pending: %d", n)
	}
	if n, next, _ := Scheduled("."); n != 1 || !next.Equal(t0.Add(DeleteGrace)) {
		t.Errorf("scheduled = %d, next %v", n, next)
	}
	if n, _ := DeleteDue(".", cfg, sp, t0.Add(DeleteGrace-time.Minute)); n != 0 || len(sp.deleted) != 0 {
		t.Fatalf("deleted before the grace period ended: %v", sp.deleted)
	}
	if n, _ := DeleteDue(".", cfg, sp, t0.Add(DeleteGrace)); n != 1 || !slices.Equal(sp.deleted, []string{"a.jpg"}) {
		t.Fatalf("want a.jpg deleted after the grace period, got %v", sp.deleted)
	}
	if _, err := os.Stat(pendingFile); !os.IsNotExist(err) {
		t.Errorf("empty queue file should be removed: %v", err)
	}
}

// A photo restored with git (or added again) is back in the catalog: its
// original must never be deleted, and its queue entry goes away.
func TestDeleteDueSkipsKeysTheCatalogUses(t *testing.T) {
	cfg, _ := newRepo(t)
	sp := &delSpy{}
	must(t, catalog.Save(cfg, []catalog.Record{{ID: "a", Original: "https://cdn.example/originals/a.jpg"}}))
	must(t, Queue(".", "a.jpg"))
	must(t, Queue(".", "b.jpg"))
	publishAll(t, t0)
	n, err := DeleteDue(".", cfg, sp, t0.Add(DeleteGrace))
	if err != nil || n != 1 || !slices.Equal(sp.deleted, []string{"b.jpg"}) {
		t.Fatalf("deleted %v (%d, %v); want only b.jpg", sp.deleted, n, err)
	}
	if keys, _ := QueuedKeys("."); len(keys) != 0 {
		t.Errorf("queue = %v, want empty", keys)
	}
}

func TestDeleteDueKeepsFailedEntries(t *testing.T) {
	cfg, _ := newRepo(t)
	sp := &delSpy{fail: "a.jpg"}
	must(t, Queue(".", "a.jpg"))
	must(t, Queue(".", "b.jpg"))
	publishAll(t, t0)
	if _, err := DeleteDue(".", cfg, sp, t0.Add(DeleteGrace)); err == nil {
		t.Fatal("want error")
	}
	keys, _ := QueuedKeys(".")
	if !slices.Contains(keys, "a.jpg") {
		t.Errorf("failed key dropped: %v", keys)
	}
}

func TestMarkPublishedKeepsEarlierDates(t *testing.T) {
	cfg, _ := newRepo(t)
	sp := &delSpy{}
	must(t, Queue(".", "a.jpg"))
	publishAll(t, t0)
	must(t, Queue(".", "b.jpg"))
	publishAll(t, t0.Add(5*24*time.Hour))
	// a.jpg is due 7 days after its own publish, not after the later one.
	n, _ := DeleteDue(".", cfg, sp, t0.Add(DeleteGrace))
	if n != 1 || !slices.Equal(sp.deleted, []string{"a.jpg"}) {
		t.Fatalf("deleted %v, want a.jpg only", sp.deleted)
	}
}

// The CLI deletes right away, but only the keys it queued itself: GUI
// deletes that haven't been published stay queued.
func TestDeleteNowOnlyGivenKeys(t *testing.T) {
	cfg, _ := newRepo(t)
	sp := &delSpy{}
	must(t, catalog.Save(cfg, []catalog.Record{{ID: "c", Original: "https://cdn.example/originals/c.jpg"}}))
	must(t, Queue(".", "gui.jpg"))
	must(t, Queue(".", "cli.jpg"))
	must(t, Queue(".", "c.jpg"))
	n, err := DeleteNow(".", cfg, sp, []string{"cli.jpg", "c.jpg"})
	if err != nil || n != 1 || !slices.Equal(sp.deleted, []string{"cli.jpg"}) {
		t.Fatalf("deleted %v (%d, %v); want cli.jpg only", sp.deleted, n, err)
	}
	if keys, _ := QueuedKeys("."); !slices.Equal(keys, []string{"gui.jpg"}) {
		t.Errorf("queue = %v, want [gui.jpg]", keys)
	}
}

// Queues written by older versions are a JSON array of keys.
func TestQueueReadsOldFormat(t *testing.T) {
	newRepo(t)
	must(t, os.WriteFile(pendingFile, []byte(`["a.jpg", "b.jpg"]`), 0o644))
	if n, _ := Count("."); n != 2 {
		t.Fatalf("pending = %d, want 2", n)
	}
	must(t, Queue(".", "c.jpg"))
	if keys, _ := QueuedKeys("."); !slices.Equal(keys, []string{"a.jpg", "b.jpg", "c.jpg"}) {
		t.Errorf("queue = %v", keys)
	}
}

// publishAll marks every queued delete as published at now.
func publishAll(t *testing.T, now time.Time) {
	t.Helper()
	keys, err := QueuedKeys(".")
	must(t, err)
	must(t, MarkPublished(".", now, keys))
}

func (d *delSpy) DeleteKey(k string) error {
	if k == d.fail {
		return errors.New("boom")
	}
	d.deleted = append(d.deleted, k)
	return nil
}

// Originals the Python tool uploaded into album subfolders must be deleted by
// their full key, not by originals/<file name>.
func TestRemoveQueuesFullKeyForSubfolderOriginal(t *testing.T) {
	cfg, st := newRepo(t)
	cfg.CDNBase = "https://cdn.example"
	must(t, catalog.Save(cfg, []catalog.Record{
		{ID: "x", Original: "https://cdn.example/originals/Greece 2026/x.jpg"},
		{ID: "y", Original: "https://cdn.example/originals/y.jpg"},
	}))
	queued, err := Remove(cfg, st, []string{"x", "y"})
	must(t, err)
	if !slices.Equal(queued, []string{"originals/Greece 2026/x.jpg", "originals/y.jpg"}) {
		t.Fatalf("queued %q", queued)
	}
	sp := &delSpy{}
	if n, err := DeleteNow(".", cfg, sp, queued); err != nil || n != 2 {
		t.Fatalf("deleted %d, %v", n, err)
	}
	if !slices.Equal(sp.deleted, []string{"originals/Greece 2026/x.jpg", "originals/y.jpg"}) {
		t.Errorf("deleted %q", sp.deleted)
	}
}

// A full key is protected by the record that uses that exact object; a bare
// file name (queued by older versions) by any record with that file name.
func TestUsedKeysFullAndLegacy(t *testing.T) {
	cfg, _ := newRepo(t)
	cfg.CDNBase = "https://cdn.example"
	must(t, catalog.Save(cfg, []catalog.Record{{ID: "x", Original: "https://cdn.example/originals/daily/x.jpg"}}))
	must(t, Queue(".", "originals/daily/x.jpg")) // in use: keep
	must(t, Queue(".", "x.jpg"))                 // legacy name of an object in use: keep
	must(t, Queue(".", "originals/other/x.jpg")) // a different object: delete
	sp := &delSpy{}
	keys, _ := QueuedKeys(".")
	if _, err := DeleteNow(".", cfg, sp, keys); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(sp.deleted, []string{"originals/other/x.jpg"}) {
		t.Errorf("deleted %q", sp.deleted)
	}
}

// A record with no original would queue "" or "."; those are never keys.
func TestQueueIgnoresEmptyKeys(t *testing.T) {
	cfg, _ := newRepo(t)
	must(t, Queue(".", ""))
	must(t, Queue(".", "."))
	if keys, _ := QueuedKeys("."); len(keys) != 0 {
		t.Fatalf("queue = %q", keys)
	}
	must(t, os.WriteFile(pendingFile, []byte(`[{"key":""},{"key":"."},{"key":"a.jpg"}]`), 0o644))
	sp := &delSpy{}
	if _, err := DeleteNow(".", cfg, sp, []string{"", ".", "a.jpg"}); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(sp.deleted, []string{"a.jpg"}) {
		t.Errorf("deleted %q", sp.deleted)
	}
	if keys, _ := QueuedKeys("."); len(keys) != 0 {
		t.Errorf("queue = %q", keys)
	}
}

// The status leaves out keys a photo in the catalog uses again (restored
// with git): they'll never be deleted, so they aren't "pending".
func TestQueueStatusIgnoresKeysInUse(t *testing.T) {
	cfg, _ := newRepo(t)
	must(t, catalog.Save(cfg, []catalog.Record{{ID: "a", Original: "https://cdn.example/originals/a.jpg"}}))
	must(t, Queue(".", "c.jpg"))
	must(t, MarkPublished(".", t0, []string{"c.jpg"}))
	must(t, Queue(".", "a.jpg")) // restored with git
	must(t, Queue(".", "b.jpg"))
	st, err := QueueStatus(".", cfg, t0.Add(DeleteGrace))
	must(t, err)
	if st.Pending != 1 || st.Scheduled != 1 || st.Due != 1 || !st.NextDue.IsZero() {
		t.Fatalf("status %+v", st)
	}
	st, _ = QueueStatus(".", cfg, t0)
	if st.Due != 0 || !st.NextDue.Equal(t0.Add(DeleteGrace)) {
		t.Fatalf("nothing is due yet: %+v", st)
	}
}
