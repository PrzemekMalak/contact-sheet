// Package app implements the catalog operations behind the photo CLI.
package app

import (
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/PrzemekMalak/contact-sheet/tool/internal/config"
	"github.com/PrzemekMalak/contact-sheet/tool/internal/store"
)

// The delete queue holds the S3 originals of deleted photos. In the GUI a
// delete is only queued; Publish marks the queued entries as published, and a
// later Publish deletes those published at least DeleteGrace ago. The delay
// covers the time the host needs to deploy, a failed build and a rollback,
// during which the live site still links to the original.
const pendingFile = ".photo-pending-deletes.json"

// DeleteGrace is how long a published delete waits before the original is
// removed from S3.
const DeleteGrace = 7 * 24 * time.Hour

type queueEntry struct {
	Key       string     `json:"key"`
	Published *time.Time `json:"published,omitempty"`
}

func readQueue(dir string) ([]queueEntry, error) {
	b, err := os.ReadFile(filepath.Join(dir, pendingFile))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var q []queueEntry
	if err := json.Unmarshal(b, &q); err == nil {
		return q, nil
	}
	// Older versions wrote a plain array of keys, all unpublished.
	var keys []string
	if err := json.Unmarshal(b, &keys); err != nil {
		return nil, err
	}
	q = nil // the failed decode above may have left zero entries
	for _, k := range keys {
		q = append(q, queueEntry{Key: k})
	}
	return q, nil
}

// writeQueue saves q, or removes the file when q is empty.
func writeQueue(dir string, q []queueEntry) error {
	p := filepath.Join(dir, pendingFile)
	if len(q) == 0 {
		if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	b, err := json.MarshalIndent(q, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, append(b, '\n'), 0o644)
}

// Queue adds key to the delete queue as unpublished. A key that is already
// queued becomes unpublished again (it was deleted again after a publish).
func Queue(dir, key string) error {
	if key == "" || key == "." {
		return nil // a record without an original has nothing to delete
	}
	q, err := readQueue(dir)
	if err != nil {
		return err
	}
	for i := range q {
		if q[i].Key == key {
			q[i].Published = nil
			return writeQueue(dir, q)
		}
	}
	return writeQueue(dir, append(q, queueEntry{Key: key}))
}

// QueuedKeys lists every queued key, published or not, in queue order.
func QueuedKeys(dir string) ([]string, error) {
	q, err := readQueue(dir)
	keys := make([]string, 0, len(q))
	for _, e := range q {
		keys = append(keys, e.Key)
	}
	return keys, err
}

// Count returns the number of deletes waiting for Publish.
func Count(dir string) (int, error) {
	q, err := readQueue(dir)
	n := 0
	for _, e := range q {
		if e.Published == nil {
			n++
		}
	}
	return n, err
}

// Scheduled returns how many published deletes are waiting for their grace
// period, and when the first of them is due (zero when none).
func Scheduled(dir string) (n int, next time.Time, err error) {
	q, err := readQueue(dir)
	for _, e := range q {
		if e.Published == nil {
			continue
		}
		n++
		due := e.Published.Add(DeleteGrace)
		if next.IsZero() || due.Before(next) {
			next = due
		}
	}
	return n, next, err
}

// QueueState summarizes the delete queue for the GUI, leaving out keys a
// catalog record uses again (they will never be deleted).
type QueueState struct {
	Pending   int       // deletes waiting for Publish
	Scheduled int       // published, waiting for DeleteGrace
	Due       int       // of Scheduled, those a Publish at now would delete
	NextDue   time.Time // when the first not-yet-due delete is due (zero if none)
}

// QueueStatus reports the queue as of now.
func QueueStatus(dir string, cfg *config.Config, now time.Time) (QueueState, error) {
	var st QueueState
	q, err := readQueue(dir)
	if err != nil || len(q) == 0 {
		return st, err
	}
	used, err := usedKeys(cfg)
	if err != nil {
		return st, err
	}
	for _, e := range q {
		if e.Key == "" || e.Key == "." || used[e.Key] {
			continue
		}
		if e.Published == nil {
			st.Pending++
			continue
		}
		st.Scheduled++
		due := e.Published.Add(DeleteGrace)
		if !now.Before(due) {
			st.Due++
			continue
		}
		if st.NextDue.IsZero() || due.Before(st.NextDue) {
			st.NextDue = due
		}
	}
	return st, nil
}

// MarkPublished stamps the unpublished entries for keys with now: the
// deletes that a publish committed. Entries queued later, and entries
// published earlier, are left as they are.
func MarkPublished(dir string, now time.Time, keys []string) error {
	q, err := readQueue(dir)
	if err != nil || len(q) == 0 {
		return err
	}
	want := map[string]bool{}
	for _, k := range keys {
		want[k] = true
	}
	now = now.UTC().Truncate(time.Second)
	for i := range q {
		if q[i].Published == nil && want[q[i].Key] {
			q[i].Published = &now
		}
	}
	return writeQueue(dir, q)
}

// DeleteDue deletes the originals published at least DeleteGrace before now.
func DeleteDue(dir string, cfg *config.Config, st store.Store, now time.Time) (int, error) {
	return process(dir, cfg, st, func(e queueEntry) bool {
		return e.Published != nil && !now.Before(e.Published.Add(DeleteGrace))
	})
}

// DeleteNow deletes the originals for keys right away (the CLI's rm and
// rm-album). Other queued entries stay, so GUI deletes that haven't been
// published aren't touched.
func DeleteNow(dir string, cfg *config.Config, st store.Store, keys []string) (int, error) {
	want := map[string]bool{}
	for _, k := range keys {
		want[k] = true
	}
	return process(dir, cfg, st, func(e queueEntry) bool { return want[e.Key] })
}

// process deletes the entries due picks. Keys a catalog record still uses (a
// photo restored with git, or added again) are dropped from the queue and
// never deleted. A failed delete keeps its entry, and the rest, for the next
// run.
func process(dir string, cfg *config.Config, st store.Store, due func(queueEntry) bool) (int, error) {
	q, err := readQueue(dir)
	if err != nil || len(q) == 0 {
		return 0, err
	}
	used, err := usedKeys(cfg)
	if err != nil {
		return 0, err
	}
	del := func(key string) error {
		if strings.Contains(key, "/") {
			return st.DeleteKey(key)
		}
		return st.Delete(key)
	}
	var keep []queueEntry
	deleted := 0
	var firstErr error
	for _, e := range q {
		switch {
		case e.Key == "" || e.Key == ".":
			// not a key: drop it
		case used[e.Key]:
			// the original is in use again: drop the entry, keep the file
		case !due(e) || firstErr != nil:
			keep = append(keep, e)
		default:
			if err := del(e.Key); err != nil {
				firstErr = err
				keep = append(keep, e)
				continue
			}
			deleted++
		}
	}
	if err := writeQueue(dir, keep); err != nil {
		return deleted, err
	}
	return deleted, firstErr
}

// objectKey is the queue key for an original: its full S3 key when the
// original is on the CDN (originals/daily/x.jpg), else its file name, which
// the store puts under the originals prefix (and local mode ignores).
func objectKey(cfg *config.Config, original string) string {
	if base := strings.TrimSuffix(cfg.CDNBase, "/"); base != "" {
		if rest, ok := strings.CutPrefix(original, base+"/"); ok && rest != "" {
			if k, err := url.PathUnescape(rest); err == nil {
				return k
			}
			return rest
		}
	}
	return path.Base(original)
}

// usedKeys is the set of queue keys the catalog still uses: each record's
// full key, and its file name, so entries older versions queued by file name
// are protected too.
func usedKeys(cfg *config.Config) (map[string]bool, error) {
	photos, err := loadCatalog(cfg)
	if err != nil {
		return nil, err
	}
	used := map[string]bool{}
	for _, r := range photos {
		if r.Original != "" {
			used[objectKey(cfg, r.Original)] = true
			used[path.Base(r.Original)] = true
		}
	}
	return used, nil
}
