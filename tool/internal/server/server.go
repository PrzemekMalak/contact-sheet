// Package server exposes the local HTTP API and embedded GUI for the photo tool.
package server

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/PrzemekMalak/contact-sheet/tool/internal/app"
	"github.com/PrzemekMalak/contact-sheet/tool/internal/catalog"
	"github.com/PrzemekMalak/contact-sheet/tool/internal/config"
	"github.com/PrzemekMalak/contact-sheet/tool/internal/content"
	"github.com/PrzemekMalak/contact-sheet/tool/internal/gitops"
	"github.com/PrzemekMalak/contact-sheet/tool/internal/store"
	"github.com/PrzemekMalak/contact-sheet/tool/web"
)

// Server is the HTTP handler. The git operations are fields so tests can
// stub them.
type Server struct {
	cfg    *config.Config
	st     store.Store
	Status func(dir string) (string, error)
	Commit func(dir, msg string) error
	Push   func(dir string) error
	// Now is the clock for scheduling S3 deletes (a field so tests can set it).
	Now func() time.Time
	mux *http.ServeMux
	// mu serializes requests that change files, so a publish never commits
	// or stamps a delete made while it runs, and a queued delete is never
	// processed between an add's upload and its catalog save.
	mu sync.Mutex
}

// New builds a Server with real git operations.
func New(cfg *config.Config, st store.Store) *Server {
	s := &Server{cfg: cfg, st: st, Status: gitops.Status, Commit: gitops.CommitAll, Push: gitops.Push, Now: time.Now, mux: http.NewServeMux()}
	m := s.mux
	m.HandleFunc("GET /api/photos", s.listPhotos)
	m.HandleFunc("POST /api/add", s.add)
	m.HandleFunc("POST /api/photos/{id}/albums", s.linkAlbums)
	m.HandleFunc("DELETE /api/photos/{id}/albums/{slug}", s.unlinkAlbum)
	m.HandleFunc("PATCH /api/photos/{id}", s.patchPhoto)
	m.HandleFunc("DELETE /api/photos/{id}", s.deletePhoto)
	m.HandleFunc("POST /api/albums", s.createAlbum)
	m.HandleFunc("PATCH /api/albums/{slug}", s.patchAlbum)
	m.HandleFunc("DELETE /api/albums/{slug}", s.deleteAlbum)
	m.HandleFunc("GET /api/status", s.status)
	m.HandleFunc("POST /api/publish", s.publish)
	m.Handle("GET /assets/gallery/", http.StripPrefix("/assets/gallery/", http.FileServer(http.Dir(cfg.AssetsDir))))
	sub, err := fs.Sub(web.FS, ".")
	if err != nil {
		panic(err)
	}
	m.Handle("GET /", http.FileServer(http.FS(sub)))
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead && strings.HasPrefix(r.URL.Path, "/api/") {
		s.mu.Lock()
		defer s.mu.Unlock()
	}
	s.mux.ServeHTTP(w, r)
}

// Run serves on 127.0.0.1:port and opens the browser.
func Run(cfg *config.Config, st store.Store, port int) error {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return err
	}
	url := "http://" + ln.Addr().String()
	fmt.Println("photo gui listening on", url)
	openBrowser(url)
	return http.Serve(ln, New(cfg, st))
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, err error) {
	code := http.StatusInternalServerError
	if strings.Contains(err.Error(), "unknown photo") {
		code = http.StatusNotFound
	}
	failCode(w, code, err)
}

func failCode(w http.ResponseWriter, code int, err error) {
	writeJSON(w, code, map[string]string{"error": err.Error()})
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		failCode(w, http.StatusBadRequest, fmt.Errorf("bad request body: %w", err))
		return false
	}
	return true
}

func (s *Server) record(w http.ResponseWriter, id string) {
	photos, err := catalog.Load(s.cfg)
	if err != nil {
		fail(w, err)
		return
	}
	rec, ok := catalog.Get(photos, id)
	if !ok {
		failCode(w, http.StatusNotFound, fmt.Errorf("unknown photo %q", id))
		return
	}
	writeJSON(w, http.StatusOK, rec)
}

func distinct(photos []catalog.Record, pick func(catalog.Record) []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, p := range photos {
		for _, v := range pick(p) {
			if !seen[v] {
				seen[v] = true
				out = append(out, v)
			}
		}
	}
	sort.Strings(out)
	return out
}

func (s *Server) listPhotos(w http.ResponseWriter, r *http.Request) {
	photos, err := catalog.Load(s.cfg)
	if err != nil {
		fail(w, err)
		return
	}
	if photos == nil {
		photos = []catalog.Record{}
	}
	albums := distinct(photos, func(p catalog.Record) []string { return p.Albums })
	// Titles for every album page, including albums with no photos yet.
	slugs := append([]string{}, albums...)
	if entries, err := os.ReadDir(s.cfg.AlbumsDir); err == nil {
		for _, e := range entries {
			if e.IsDir() && app.ValidAlbumSlug(e.Name()) == nil {
				slugs = append(slugs, e.Name())
			}
		}
	}
	titles := map[string]string{}
	for _, a := range slugs {
		if _, done := titles[a]; done {
			continue
		}
		t, err := content.AlbumTitle(s.cfg, a)
		if err != nil {
			fail(w, err)
			return
		}
		if t != "" {
			titles[a] = t
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"photos":      photos,
		"albums":      albums,
		"albumTitles": titles,
		"tags":        distinct(photos, func(p catalog.Record) []string { return p.Tags }),
	})
}

func splitList(vals []string) []string {
	out := []string{}
	for _, v := range vals {
		for _, p := range strings.Split(v, ",") {
			if p = strings.TrimSpace(p); p != "" {
				out = append(out, p)
			}
		}
	}
	return out
}

func (s *Server) add(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		failCode(w, http.StatusBadRequest, err)
		return
	}
	tmp, err := os.MkdirTemp("", "photo-upload-")
	if err != nil {
		fail(w, err)
		return
	}
	defer os.RemoveAll(tmp)
	var paths []string
	for _, fh := range r.MultipartForm.File["files"] {
		src, err := fh.Open()
		if err != nil {
			fail(w, err)
			return
		}
		dst := filepath.Join(tmp, filepath.Base(fh.Filename))
		out, err := os.Create(dst)
		if err != nil {
			src.Close()
			fail(w, err)
			return
		}
		_, err = io.Copy(out, src)
		src.Close()
		if cerr := out.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			fail(w, err)
			return
		}
		paths = append(paths, dst)
	}
	if len(paths) == 0 {
		failCode(w, http.StatusBadRequest, fmt.Errorf("no files uploaded"))
		return
	}
	recs, err := app.Add(s.cfg, s.st, paths, app.AddOpts{
		Albums:   splitList(r.MultipartForm.Value["albums"]),
		Tags:     splitList(r.MultipartForm.Value["tags"]),
		Featured: r.FormValue("featured") == "true",
		WithExif: true,
	})
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, recs)
}

func (s *Server) linkAlbums(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Albums []string `json:"albums"`
	}
	if !decode(w, r, &body) {
		return
	}
	for _, slug := range body.Albums {
		if err := app.Link(s.cfg, slug, []string{id}); err != nil {
			fail(w, err)
			return
		}
	}
	s.record(w, id)
}

func (s *Server) unlinkAlbum(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := app.Unlink(s.cfg, r.PathValue("slug"), []string{id}); err != nil {
		fail(w, err)
		return
	}
	s.record(w, id)
}

func (s *Server) patchPhoto(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Caption  *string   `json:"caption"`
		Featured *bool     `json:"featured"`
		Tags     *[]string `json:"tags"`
	}
	if !decode(w, r, &body) {
		return
	}
	photos, err := catalog.Load(s.cfg)
	if err != nil {
		fail(w, err)
		return
	}
	rec, ok := catalog.Get(photos, id)
	if !ok {
		failCode(w, http.StatusNotFound, fmt.Errorf("unknown photo %q", id))
		return
	}
	if body.Caption != nil {
		rec.Caption = *body.Caption
	}
	if body.Featured != nil {
		rec.Featured = *body.Featured
	}
	if body.Tags != nil {
		rec.Tags = normTags(*body.Tags)
	}
	updated := catalog.Upsert(photos, rec)
	if err := catalog.Save(s.cfg, updated); err != nil {
		fail(w, err)
		return
	}
	if body.Tags != nil {
		// Tags changed: keep the Hugo tag stub pages in sync.
		if err := content.RebuildTagStubs(s.cfg, updated); err != nil {
			fail(w, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, rec)
}

// normTags trims, drops blanks, and de-duplicates (preserving order). Always
// returns a non-nil slice so it serializes as [] rather than null.
func normTags(in []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, t := range in {
		t = strings.TrimSpace(t)
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	return out
}

// deletePhoto removes the photo locally; the S3 delete is only queued and
// runs after a successful publish.
func (s *Server) deletePhoto(w http.ResponseWriter, r *http.Request) {
	if _, err := app.Remove(s.cfg, s.st, []string{r.PathValue("id")}); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) createAlbum(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Slug, Title, Cover string
	}
	if !decode(w, r, &body) {
		return
	}
	if body.Slug == "" {
		failCode(w, http.StatusBadRequest, fmt.Errorf("slug required"))
		return
	}
	if err := app.ValidAlbumSlug(body.Slug); err != nil {
		failCode(w, http.StatusBadRequest, err)
		return
	}
	if err := content.EnsureAlbum(s.cfg, body.Slug, body.Title, body.Cover); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) patchAlbum(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Cover *string `json:"cover"`
		Title *string `json:"title"`
	}
	if !decode(w, r, &body) {
		return
	}
	slug := r.PathValue("slug")
	if err := app.ValidAlbumSlug(slug); err != nil {
		failCode(w, http.StatusBadRequest, err)
		return
	}
	if body.Cover == nil && body.Title == nil {
		failCode(w, http.StatusBadRequest, fmt.Errorf("nothing to change: send cover or title"))
		return
	}
	if body.Title != nil {
		t := strings.TrimSpace(*body.Title)
		if t == "" {
			failCode(w, http.StatusBadRequest, fmt.Errorf("title can't be empty"))
			return
		}
		if err := content.SetTitle(s.cfg, slug, t); err != nil {
			fail(w, err)
			return
		}
	}
	if body.Cover != nil {
		if err := content.SetCover(s.cfg, slug, *body.Cover); err != nil {
			fail(w, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// deleteAlbum removes every photo from the album and deletes its page. With
// ?purge=true it also deletes the photos left in no album; their S3 originals
// are only queued, and Publish deletes them after a successful push.
func (s *Server) deleteAlbum(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if err := app.ValidAlbumSlug(slug); err != nil {
		failCode(w, http.StatusBadRequest, err)
		return
	}
	unlinked, orphans, purged, _, err := app.RemoveAlbum(s.cfg, s.st, slug, r.URL.Query().Get("purge") == "true")
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"unlinked": unlinked, "orphans": orphans, "purged": purged})
}

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	q, err := app.QueueStatus(".", s.cfg, s.Now())
	if err != nil {
		fail(w, err)
		return
	}
	out := map[string]any{"pendingDeletes": q.Pending, "scheduledDeletes": q.Scheduled,
		"dueDeletes": q.Due, "site": s.cfg.SiteTitle}
	if !q.NextDue.IsZero() {
		out["nextDeleteDue"] = q.NextDue.Format(time.RFC3339)
	}
	// A git failure must not hide the pending count; report it alongside.
	if g, err := s.Status("."); err != nil {
		out["git"], out["gitError"] = "", err.Error()
	} else {
		out["git"] = g
	}
	writeJSON(w, http.StatusOK, out)
}

// publish commits and pushes. Only after both succeed are the queued S3
// deletes marked as published; they run on a Publish app.DeleteGrace later.
func (s *Server) publish(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Message string `json:"message"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.Message == "" {
		body.Message = "update gallery"
	}
	// The deletes this publish commits: the ones queued before it started.
	keys, err := app.QueuedKeys(".")
	if err != nil {
		failCode(w, http.StatusInternalServerError, err)
		return
	}
	// Normalize + rebuild derived content (tag/gear stubs) before committing,
	// so the published state is always canonical and complete. Idempotent.
	if err := app.Sync(s.cfg); err != nil {
		failCode(w, http.StatusInternalServerError, fmt.Errorf("sync failed: %w", err))
		return
	}
	// With nothing to commit (only deletes that are due, or commits made
	// with git directly) there's no commit, but the push and the deletes run.
	changes, err := s.Status(".")
	if err != nil {
		failCode(w, http.StatusInternalServerError, err)
		return
	}
	if strings.TrimSpace(changes) != "" {
		if err := s.Commit(".", body.Message); err != nil {
			failCode(w, http.StatusInternalServerError, err)
			return
		}
	}
	if err := s.Push("."); err != nil {
		failCode(w, http.StatusInternalServerError, err)
		return
	}
	// The deletes this publish queued wait app.DeleteGrace: the live site may
	// still link to them until the host deploys, or after a rollback. Earlier
	// ones whose time has come are deleted now.
	now := s.Now()
	if err := app.MarkPublished(".", now, keys); err != nil {
		failCode(w, http.StatusInternalServerError, fmt.Errorf("pushed, but couldn't schedule the S3 deletes: %w", err))
		return
	}
	n, err := app.DeleteDue(".", s.cfg, s.st, now)
	if err != nil {
		failCode(w, http.StatusInternalServerError, fmt.Errorf("pushed, but S3 cleanup failed (queue kept): %w", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": body.Message, "deletedFromS3": n})
}
