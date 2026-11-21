package cdk

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeSite(t *testing.T, toml string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "contact-sheet.toml"), []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestLoadSettings(t *testing.T) {
	dir := writeSite(t, `
[s3]
bucket = "b"

[cdk]
infra = "../cs/infra"
profile = "p"

[cdk.context]
site_name = "a.com"
budget_limit = 10
ratio = 12.5
flag = true
`)
	s, err := LoadSettings(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := Settings{Infra: "../cs/infra", Profile: "p", Context: map[string]string{
		"site_name": "a.com", "budget_limit": "10", "ratio": "12.5", "flag": "true",
	}}
	if !reflect.DeepEqual(s, want) {
		t.Fatalf("got %+v, want %+v", s, want)
	}
}

func TestLoadSettingsNoSection(t *testing.T) {
	s, err := LoadSettings(writeSite(t, "[s3]\nbucket = \"\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(s, Settings{}) {
		t.Fatalf("got %+v, want zero Settings", s)
	}
}

func TestLoadSettingsRejects(t *testing.T) {
	cases := []struct {
		name, toml string
		want       []string
	}{
		{"array", "[cdk.context]\nx = [1, 2]\n", []string{`"x"`}},
		{"table", "[cdk.context.y]\nz = 1\n", []string{`"y"`}},
		{"todo", "[cdk.context]\na = \"TODO\"\nb = \"TODO: x\"\nc = \"ok\"\n", []string{"fill in or delete", "a", "b"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := LoadSettings(writeSite(t, c.toml))
			if err == nil {
				t.Fatal("want error")
			}
			for _, w := range c.want {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("error %q does not mention %s", err, w)
				}
			}
		})
	}
}

func TestLoadSettingsMissingFile(t *testing.T) {
	_, err := LoadSettings(t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "contact-sheet.toml") {
		t.Fatalf("want error mentioning contact-sheet.toml, got %v", err)
	}
}

func TestResolveInfra(t *testing.T) {
	site, home := "/site/root", "/home/me"
	s := Settings{Infra: "../cs/infra"}
	cases := []struct {
		name, flag, env string
		s               Settings
		want            string
	}{
		{"flag wins", "/flag/infra", "/env/infra", s, "/flag/infra"},
		{"env wins over settings", "", "/env/infra", s, "/env/infra"},
		{"settings relative to site", "", "", s, filepath.Clean("/site/cs/infra")},
		{"tilde", "", "", Settings{Infra: "~/cs/infra"}, filepath.Join(home, "cs/infra")},
		{"absolute settings", "", "", Settings{Infra: "/abs/infra"}, "/abs/infra"},
		{"relative flag is relative to site", "rel/infra", "", Settings{}, filepath.Join(site, "rel/infra")},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ResolveInfra(c.flag, c.env, site, home, c.s)
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want {
				t.Fatalf("got %q, want %q", got, c.want)
			}
		})
	}
	_, err := ResolveInfra("", "", site, home, Settings{})
	if err == nil || !strings.Contains(err.Error(), "CONTACT_SHEET_INFRA") || !strings.Contains(err.Error(), "git clone") {
		t.Fatalf("want error explaining clone + CONTACT_SHEET_INFRA, got %v", err)
	}
}

// makeInfra creates an infra dir with the given relative files.
func makeInfra(t *testing.T, files ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, f := range files {
		p := filepath.Join(dir, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func okLookPath(string) (string, error) { return "/usr/bin/npx", nil }

func TestPreflight(t *testing.T) {
	cases := []struct {
		name  string
		goos  string
		files []string
		look  func(string) (string, error)
		want  string // "" = no error
	}{
		{"ok", "darwin", []string{"cdk.json", "app.py", ".venv/bin/python"}, okLookPath, ""},
		{"ok windows", "windows", []string{"cdk.json", "app.py", ".venv/Scripts/python.exe"}, okLookPath, ""},
		{"no cdk.json", "darwin", []string{"app.py", ".venv/bin/python"}, okLookPath, "cdk.json"},
		{"no app.py", "darwin", []string{"cdk.json", ".venv/bin/python"}, okLookPath, "app.py"},
		{"no venv", "darwin", []string{"cdk.json", "app.py"}, okLookPath, "python3 -m venv .venv"},
		{"windows venv missing", "windows", []string{"cdk.json", "app.py", ".venv/bin/python"}, okLookPath, "python3 -m venv .venv"},
		{"no npx", "darwin", []string{"cdk.json", "app.py", ".venv/bin/python"},
			func(string) (string, error) { return "", errors.New("not found") }, "npx"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := Preflight(makeInfra(t, c.files...), c.goos, c.look)
			if c.want == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("want error mentioning %q, got %v", c.want, err)
			}
		})
	}
}

func TestArgs(t *testing.T) {
	got := Args([]string{"deploy", "S"}, "darwin", map[string]string{"b": "2", "a": "x=y z"})
	want := []string{"--yes", "aws-cdk@latest", "--app", ".venv/bin/python app.py",
		"-c", "a=x=y z", "-c", "b=2", "deploy", "S"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q\nwant %q", got, want)
	}
	win := Args([]string{"diff"}, "windows", nil)
	wantWin := []string{"--yes", "aws-cdk@latest", "--app", `.venv\Scripts\python.exe app.py`, "diff"}
	if !reflect.DeepEqual(win, wantWin) {
		t.Fatalf("got %q\nwant %q", win, wantWin)
	}
}

// CDK keeps the last value of a repeated -c key, so the user's own -c must
// come after the toml values to win.
func TestArgsCommandLineContextWins(t *testing.T) {
	got := Args([]string{"deploy", "S", "-c", "price_class=PriceClass_All"}, "darwin",
		map[string]string{"price_class": "PriceClass_100"})
	last := -1
	for i, a := range got {
		if a == "price_class=PriceClass_100" {
			if last != -1 {
				t.Fatalf("toml value after the user's: %q", got)
			}
		}
		if a == "price_class=PriceClass_All" {
			last = i
		}
	}
	if last != len(got)-1 {
		t.Fatalf("user's -c must be the last price_class value: %q", got)
	}
}

func TestEnv(t *testing.T) {
	base := []string{"PATH=/bin", "HOME=/h"}
	got := Env(base, "p")
	if !contains(got, "AWS_PROFILE=p") || !contains(got, "PATH=/bin") {
		t.Fatalf("want AWS_PROFILE=p added, got %q", got)
	}
	set := []string{"PATH=/bin", "AWS_PROFILE=q"}
	if got := Env(set, "p"); !reflect.DeepEqual(got, set) {
		t.Fatalf("env AWS_PROFILE must win, got %q", got)
	}
	if got := Env(base, ""); !reflect.DeepEqual(got, base) {
		t.Fatalf("empty profile must not change env, got %q", got)
	}
}

func TestSummary(t *testing.T) {
	s := Summary("/x/infra", "prof", map[string]string{"b": "secret@example.com", "a": "x=y z"})
	for _, w := range []string{"/x/infra", "prof", "a, b"} {
		if !strings.Contains(s, w) {
			t.Errorf("summary %q lacks %q", s, w)
		}
	}
	for _, v := range []string{"secret@example.com", "x=y z"} {
		if strings.Contains(s, v) {
			t.Errorf("summary %q leaks value %q", s, v)
		}
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func TestArgsKeepsTheUsersApp(t *testing.T) {
	for _, user := range [][]string{{"synth", "--app", "cdk.out"}, {"synth", "-a", "cdk.out"}, {"synth", "--app=cdk.out"}} {
		got := Args(user, "darwin", nil)
		n := 0
		for _, a := range got {
			if a == "--app" || a == "-a" || strings.HasPrefix(a, "--app=") {
				n++
			}
		}
		if n != 1 || contains(got, ".venv/bin/python app.py") {
			t.Errorf("%q: want only the user's --app, got %q", user, got)
		}
	}
}

func TestEnvEmptyProfileCountsAsUnset(t *testing.T) {
	got := Env([]string{"PATH=/bin", "AWS_PROFILE="}, "p")
	if !contains(got, "AWS_PROFILE=p") || contains(got, "AWS_PROFILE=") {
		t.Fatalf("got %q", got)
	}
}
