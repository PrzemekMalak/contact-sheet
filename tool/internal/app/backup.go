package app

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/PrzemekMalak/contact-sheet/tool/internal/catalog"
	"github.com/PrzemekMalak/contact-sheet/tool/internal/config"
)

// BackupOpts tunes Backup. Zero values pick sensible defaults.
type BackupOpts struct {
	Client *http.Client                     // HTTP client for downloads (default: 5 min timeout)
	Jobs   int                              // parallel downloads (default 4)
	Log    func(format string, args ...any) // progress lines (default: silent)
}

// BackupResult summarises a Backup run.
type BackupResult struct {
	Downloaded int      // new files fetched this run
	Skipped    int      // already present in dest with the same size (or size unknown)
	Replaced   int      // present in dest but with a different size: downloaded again
	NotRemote  int      // records whose original is not an http(s) URL (local mode; lives in git)
	Extra      int      // files in dest that no catalog record points at (kept, never deleted)
	Bytes      int64    // bytes downloaded this run
	Failed     []string // "<id>: <error>" per failed download
}

// Backup copies the original of every photo in the catalog into dest by
// downloading its public CDN URL, so it needs no AWS credentials. Objects in
// the bucket that no photo uses are not copied. It is incremental: a file
// already present is checked with a HEAD request and downloaded again only if
// its size differs (a different photo got the same file name); when the size
// can't be checked, the file is kept. It writes via a temp file + rename so an
// interrupted download never looks complete, never deletes anything, and
// stores a snapshot of the catalog as dest/photos.json.
func Backup(cfg *config.Config, dest string, opts BackupOpts) (BackupResult, error) {
	var res BackupResult
	photos, err := catalog.Load(cfg)
	if err != nil {
		return res, err
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return res, err
	}
	if opts.Client == nil {
		opts.Client = &http.Client{Timeout: 5 * time.Minute}
	}
	if opts.Jobs < 1 {
		opts.Jobs = 4
	}
	logf := opts.Log
	if logf == nil {
		logf = func(string, ...any) {}
	}

	// existing is the local file's size, or -1 when there's none.
	type job struct {
		id, url, name string
		existing      int64
	}
	var jobs []job
	wanted := map[string]bool{"photos.json": true}
	for _, p := range photos {
		u, err := url.Parse(p.Original)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
			res.NotRemote++
			continue
		}
		name := path.Base(u.Path)
		if name == "." || name == "/" || name == "" {
			res.Failed = append(res.Failed, fmt.Sprintf("%s: cannot derive a file name from %q", p.ID, p.Original))
			continue
		}
		wanted[name] = true
		existing := int64(-1)
		if fi, err := os.Stat(filepath.Join(dest, name)); err == nil && fi.Size() > 0 {
			existing = fi.Size()
		}
		jobs = append(jobs, job{p.ID, p.Original, name, existing})
	}

	var mu sync.Mutex
	ch := make(chan job)
	var wg sync.WaitGroup
	done := 0
	for i := 0; i < opts.Jobs; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range ch {
				if j.existing >= 0 {
					if size, ok := remoteSize(opts.Client, j.url); !ok || size == j.existing {
						mu.Lock()
						done++
						res.Skipped++
						mu.Unlock()
						continue
					}
				}
				n, err := download(opts.Client, j.url, filepath.Join(dest, j.name))
				mu.Lock()
				done++
				switch {
				case err != nil:
					res.Failed = append(res.Failed, fmt.Sprintf("%s: %v", j.id, err))
					logf("[%d/%d] FAILED %s: %v", done, len(jobs), j.name, err)
				case j.existing >= 0:
					res.Replaced++
					res.Bytes += n
					logf("[%d/%d] %s replaced, size changed (%.1f MB)", done, len(jobs), j.name, float64(n)/1e6)
				default:
					res.Downloaded++
					res.Bytes += n
					logf("[%d/%d] %s (%.1f MB)", done, len(jobs), j.name, float64(n)/1e6)
				}
				mu.Unlock()
			}
		}()
	}
	for _, j := range jobs {
		ch <- j
	}
	close(ch)
	wg.Wait()
	sort.Strings(res.Failed)

	// Catalog snapshot, so the backup is self-describing (captions, albums, EXIF).
	if b, err := os.ReadFile(cfg.PhotosIndex); err == nil {
		if err := writeAtomic(filepath.Join(dest, "photos.json"), b); err != nil {
			return res, err
		}
	} else {
		return res, err
	}

	entries, err := os.ReadDir(dest)
	if err != nil {
		return res, err
	}
	for _, e := range entries {
		if !e.IsDir() && !wanted[e.Name()] && !strings.HasSuffix(e.Name(), ".part") && !strings.HasPrefix(e.Name(), ".") {
			res.Extra++
		}
	}
	return res, nil
}

// remoteSize returns the size the CDN reports for src, if it reports one.
func remoteSize(c *http.Client, src string) (int64, bool) {
	resp, err := c.Head(src)
	if err != nil {
		return 0, false
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.ContentLength < 0 {
		return 0, false
	}
	return resp.ContentLength, true
}

func download(c *http.Client, src, dst string) (int64, error) {
	resp, err := c.Get(src)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("HTTP %s", resp.Status)
	}
	tmp := dst + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return 0, err
	}
	n, err := io.Copy(f, resp.Body)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil && resp.ContentLength >= 0 && n != resp.ContentLength {
		err = fmt.Errorf("short read: got %d of %d bytes", n, resp.ContentLength)
	}
	if err == nil && n == 0 {
		err = fmt.Errorf("empty response")
	}
	if err != nil {
		os.Remove(tmp)
		return 0, err
	}
	return n, os.Rename(tmp, dst)
}

func writeAtomic(dst string, b []byte) error {
	tmp := dst + ".part"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}
