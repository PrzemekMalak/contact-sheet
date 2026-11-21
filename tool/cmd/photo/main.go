// Command photo manages a contact-sheet photo catalog.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"

	"github.com/PrzemekMalak/contact-sheet/tool/internal/app"
	"github.com/PrzemekMalak/contact-sheet/tool/internal/cdk"
	"github.com/PrzemekMalak/contact-sheet/tool/internal/config"
	"github.com/PrzemekMalak/contact-sheet/tool/internal/scaffold"
	"github.com/PrzemekMalak/contact-sheet/tool/internal/server"
	"github.com/PrzemekMalak/contact-sheet/tool/internal/store"
	"github.com/PrzemekMalak/contact-sheet/tool/internal/theme"
)

// version is set by release builds (-ldflags "-X main.version=vX.Y.Z").
var version = "dev"

// toolVersion is the release tag, the module version for `go install pkg@vX`
// builds, or "dev".
func toolVersion() string {
	if version != "dev" {
		return version
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return bi.Main.Version
	}
	return version
}

const usage = `usage: photo <command> [args]

commands:
  init <dir> [--title T] [--url URL] [--author A] [--amplify] [--module]
  theme update [--version vX.Y.Z]
  add <paths...> [-a album]... [--tags a,b] [--featured] [--no-exif] [-r]
  link <album> <id...>
  unlink <album> <id...>
  rm <id...> [--yes]
  rm-album <slug> [--purge-orphans] [--yes]
  retag <id> --tags a,b
  sync
  backup <dir> [--jobs N]
  gui [--port N]
  cdk [--infra DIR] <diff|deploy STACK|synth|...>
  version

Run photo <command> -h for a command's options. Full reference: docs/cli.md
in https://github.com/PrzemekMalak/contact-sheet
`

type multi []string

func (m *multi) String() string     { return strings.Join(*m, ",") }
func (m *multi) Set(v string) error { *m = append(*m, v); return nil }

func splitTags(s string) []string {
	out := []string{}
	for _, t := range strings.Split(s, ",") {
		if t = strings.TrimSpace(t); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// parse parses flags allowing them to appear after positional arguments.
func parse(fs *flag.FlagSet, args []string) ([]string, error) {
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		args = fs.Args()
		if len(args) == 0 {
			return pos, nil
		}
		pos = append(pos, args[0])
		args = args[1:]
	}
}

func main() { os.Exit(run(os.Args[1:])) }

func run(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usage)
		return 2
	}
	cmd, rest := args[0], args[1:]
	if isHelpFlag(cmd) || cmd == "help" {
		fmt.Fprint(helpOut, usage)
		return 0
	}
	if h, ok := commandHelp[cmd]; ok && wantsHelp(cmd, rest) {
		fmt.Fprint(helpOut, h)
		return 0
	}
	switch cmd {
	case "version":
		fmt.Println("photo", toolVersion())
		return 0
	case "cdk":
		return runCDK(rest)
	case "theme":
		if err := runTheme(rest); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return 1
		}
		return 0
	case "init", "add", "link", "unlink", "rm", "rm-album", "retag", "sync", "backup", "gui":
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", cmd, usage)
		return 2
	}
	if cmd == "init" {
		if err := runInit(rest); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return 1
		}
		return 0
	}
	if cmd == "gui" {
		if err := runGUI(rest); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return 1
		}
		return 0
	}
	if cmd == "backup" {
		if err := runBackup(rest); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return 1
		}
		return 0
	}
	if err := dispatch(cmd, rest); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	return 0
}

// runCDK runs contact-sheet's CDK app with the site's [cdk] settings and
// returns CDK's own exit code.
func runCDK(args []string) int {
	site, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	home, _ := os.UserHomeDir()
	code, err := cdk.Run(args, cdk.Options{
		SiteDir: site, Home: home, GOOS: runtime.GOOS, Environ: os.Environ(),
		LookPath: exec.LookPath, Exec: cdk.OSExec, Log: os.Stderr,
	})
	if errors.Is(err, cdk.ErrUsage) {
		fmt.Fprintln(os.Stderr, err)
	} else if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
	}
	return code
}

// runInit creates a new site; it runs before any config exists.
func runInit(args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	var o scaffold.Options
	fs.StringVar(&o.Title, "title", "", "site title (default: the directory name)")
	fs.StringVar(&o.BaseURL, "url", "", "site URL, e.g. https://photos.example.com/")
	fs.StringVar(&o.Author, "author", "", "name in the footer (default: the title)")
	fs.BoolVar(&o.Amplify, "amplify", false, "also write amplify.yml for AWS Amplify hosting")
	fs.BoolVar(&o.Module, "module", false, "import the theme as a Hugo Module (needs Go) instead of copying it into themes/")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return fmt.Errorf("init: need exactly one <dir>")
	}
	dir := pos[0]
	_, statErr := os.Stat(dir)
	created, err := scaffold.Init(dir, o)
	if err != nil {
		return err
	}
	if !o.Module {
		src := theme.Source(toolVersion())
		fmt.Println("downloading theme from", src)
		v, err := theme.Install(dir, src)
		if err != nil {
			// Don't leave a half-made site behind.
			if os.IsNotExist(statErr) {
				os.RemoveAll(dir)
			} else {
				for _, f := range created {
					os.Remove(filepath.Join(dir, f))
				}
			}
			return err
		}
		created = append(created, "themes/"+theme.Name+"/ ("+v+")")
	}
	for _, f := range created {
		fmt.Println("  created", f)
	}
	serve := "hugo server"
	if o.Module {
		serve = "hugo server                                  # first run downloads the theme"
	}
	fmt.Printf(`
Next:
  cd %s
  photo add ~/Pictures/some-trip -a some-trip   # or: photo gui
  %s

Edit hugo.toml for the title, URL and author, and content/ for your pages.
`, dir, serve)
	return nil
}

// runTheme handles `photo theme update`: replace the theme copy in themes/.
func runTheme(args []string) error {
	if len(args) == 0 || args[0] != "update" {
		return fmt.Errorf("usage: photo theme update [--version vX.Y.Z]")
	}
	fs := flag.NewFlagSet("theme update", flag.ContinueOnError)
	want := fs.String("version", "latest", "theme release to install, e.g. v0.2.0")
	if _, err := parse(fs, args[1:]); err != nil {
		return err
	}
	if _, err := os.Stat("hugo.toml"); err != nil {
		return fmt.Errorf("no hugo.toml here; run photo theme update from your site's root")
	}
	old, err := theme.InstalledVersion(".")
	if err != nil {
		if b, _ := os.ReadFile("hugo.toml"); strings.Contains(string(b), scaffold.ThemeModule) {
			return fmt.Errorf("this site imports the theme as a Hugo Module; update it with: hugo mod get -u")
		}
		return fmt.Errorf("no theme copy in themes/%s: %w", theme.Name, err)
	}
	src := theme.Source(*want)
	fmt.Println("downloading theme from", src)
	v, err := theme.Install(".", src)
	if err != nil {
		return err
	}
	if v == old {
		fmt.Println("theme already at", v)
	} else {
		fmt.Printf("theme updated: %s -> %s\n", old, v)
	}
	return nil
}

func runGUI(args []string) error {
	fs := flag.NewFlagSet("gui", flag.ContinueOnError)
	port := fs.Int("port", 8765, "port to listen on (127.0.0.1)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := config.Load(".")
	if err != nil {
		return err
	}
	st, err := store.New(cfg)
	if err != nil {
		return err
	}
	return server.Run(cfg, st, *port)
}

// runBackup mirrors originals from the public CDN into a local directory.
// It needs no AWS credentials, so it skips store setup entirely.
func runBackup(args []string) error {
	fs := flag.NewFlagSet("backup", flag.ContinueOnError)
	jobs := fs.Int("jobs", 4, "parallel downloads")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return fmt.Errorf("backup: need exactly one <dir>")
	}
	cfg, err := config.Load(".")
	if err != nil {
		return err
	}
	res, err := app.Backup(cfg, pos[0], app.BackupOpts{
		Jobs: *jobs,
		Log:  func(f string, a ...any) { fmt.Printf(f+"\n", a...) },
	})
	if err != nil {
		return err
	}
	fmt.Printf("backup %s: %d downloaded (%.1f MB), %d already present", pos[0], res.Downloaded, float64(res.Bytes)/1e6, res.Skipped)
	if res.Replaced > 0 {
		fmt.Printf(", %d replaced (size changed)", res.Replaced)
	}
	if res.NotRemote > 0 {
		fmt.Printf(", %d local-only (in git)", res.NotRemote)
	}
	if res.Extra > 0 {
		fmt.Printf(", %d extra file(s) kept", res.Extra)
	}
	fmt.Println()
	if len(res.Failed) > 0 {
		for _, f := range res.Failed {
			fmt.Fprintln(os.Stderr, "  failed:", f)
		}
		return fmt.Errorf("%d download(s) failed; rerun to retry", len(res.Failed))
	}
	return nil
}

// deleteNow runs remove, which queues S3 deletes and returns their keys, and
// deletes those originals right away: in a script you've already decided.
// Anything else in the queue (GUI deletes not yet published) stays.
func deleteNow(cfg *config.Config, st store.Store, remove func() ([]string, error)) error {
	keys, err := remove()
	if len(keys) > 0 {
		if _, derr := app.DeleteNow(".", cfg, st, keys); err == nil {
			err = derr
		}
	}
	return err
}

func dispatch(cmd string, args []string) error {
	cfg, err := config.Load(".")
	if err != nil {
		return err
	}
	st, err := store.New(cfg)
	if err != nil {
		return err
	}
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	switch cmd {
	case "add":
		var albums multi
		var tags string
		var featured, noExif, rec bool
		fs.Var(&albums, "a", "album slug (repeatable)")
		fs.StringVar(&tags, "tags", "", "comma-separated tags")
		fs.BoolVar(&featured, "featured", false, "mark featured")
		fs.BoolVar(&noExif, "no-exif", false, "skip EXIF extraction")
		fs.BoolVar(&rec, "r", false, "recurse into directories")
		pos, err := parse(fs, args)
		if err != nil {
			return err
		}
		if len(pos) == 0 {
			return fmt.Errorf("add: no paths given")
		}
		recs, err := app.Add(cfg, st, pos, app.AddOpts{
			Tags: splitTags(tags), Albums: albums, Featured: featured, WithExif: !noExif, Recursive: rec,
		})
		if err != nil {
			return err
		}
		fmt.Printf("processed %d photo(s)\n", len(recs))
	case "link", "unlink":
		pos, err := parse(fs, args)
		if err != nil {
			return err
		}
		if len(pos) < 2 {
			return fmt.Errorf("%s: need <album> <id...>", cmd)
		}
		if cmd == "link" {
			return app.Link(cfg, pos[0], pos[1:])
		}
		return app.Unlink(cfg, pos[0], pos[1:])
	case "rm":
		fs.Bool("yes", false, "skip confirmation (no-op)")
		pos, err := parse(fs, args)
		if err != nil {
			return err
		}
		if len(pos) == 0 {
			return fmt.Errorf("rm: no ids given")
		}
		return deleteNow(cfg, st, func() ([]string, error) { return app.Remove(cfg, st, pos) })
	case "rm-album":
		purge := fs.Bool("purge-orphans", false, "also delete photos left in no album")
		fs.Bool("yes", false, "skip confirmation (no-op)")
		pos, err := parse(fs, args)
		if err != nil {
			return err
		}
		if len(pos) != 1 {
			return fmt.Errorf("rm-album: need exactly one <slug>")
		}
		return deleteNow(cfg, st, func() ([]string, error) {
			u, o, p, queued, err := app.RemoveAlbum(cfg, st, pos[0], *purge)
			if err == nil {
				fmt.Printf("unlinked %d, orphans %d, purged %d\n", u, o, p)
			}
			return queued, err
		})
	case "retag":
		tags := fs.String("tags", "", "comma-separated tags")
		pos, err := parse(fs, args)
		if err != nil {
			return err
		}
		if len(pos) != 1 {
			return fmt.Errorf("retag: need exactly one <id>")
		}
		return app.Retag(cfg, pos[0], splitTags(*tags))
	case "sync":
		return app.Sync(cfg)
	}
	return nil
}
