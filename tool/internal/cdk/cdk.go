// Package cdk runs contact-sheet's AWS CDK app (infra/ in a local clone of
// the repo) with per-site settings from the [cdk] section of the site's
// contact-sheet.toml, so those settings live in the site repo instead of in
// the clone's cdk.json.
package cdk

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
)

// configFile is the site's tool config; [cdk] lives next to [s3] in it.
const configFile = "contact-sheet.toml"

// Settings is the [cdk] section of contact-sheet.toml.
type Settings struct {
	// Infra is the path to contact-sheet's infra/ directory as written:
	// absolute, relative to the site root, or starting with ~.
	Infra string
	// Profile is the AWS profile that can deploy (not the [s3] upload key).
	Profile string
	// Context holds the [cdk.context] values as text, passed as -c key=value.
	Context map[string]string
}

// LoadSettings reads the [cdk] section of <siteDir>/contact-sheet.toml. A
// file without the section gives zero Settings.
func LoadSettings(siteDir string) (Settings, error) {
	path := filepath.Join(siteDir, configFile)
	var f struct {
		CDK struct {
			Infra   string         `toml:"infra"`
			Profile string         `toml:"profile"`
			Context map[string]any `toml:"context"`
		} `toml:"cdk"`
	}
	if _, err := toml.DecodeFile(path, &f); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Settings{}, fmt.Errorf("no %s in %s (run photo from the site root)", configFile, siteDir)
		}
		return Settings{}, fmt.Errorf("load %s: %w", path, err)
	}
	s := Settings{Infra: f.CDK.Infra, Profile: f.CDK.Profile}
	if len(f.CDK.Context) == 0 {
		return s, nil
	}
	s.Context = make(map[string]string, len(f.CDK.Context))
	var todo []string
	for _, k := range sortedKeys(f.CDK.Context) {
		var v string
		switch x := f.CDK.Context[k].(type) {
		case string:
			v = x
		case int64:
			v = strconv.FormatInt(x, 10)
		case float64:
			v = strconv.FormatFloat(x, 'f', -1, 64)
		case bool:
			v = strconv.FormatBool(x)
		default:
			return Settings{}, fmt.Errorf("%s: [cdk.context] %q must be a string, number or boolean", configFile, k)
		}
		if strings.HasPrefix(v, "TODO") {
			todo = append(todo, k)
		}
		s.Context[k] = v
	}
	if len(todo) > 0 {
		return Settings{}, fmt.Errorf("%s: fill in or delete [cdk.context] %s", configFile, strings.Join(todo, ", "))
	}
	return s, nil
}

// ResolveInfra picks the infra directory: the --infra flag, then the
// CONTACT_SHEET_INFRA environment variable, then [cdk] infra. A leading ~ is
// the home directory; other relative paths are relative to the site root.
func ResolveInfra(flagVal, envVal, siteDir, home string, s Settings) (string, error) {
	p := flagVal
	if p == "" {
		p = envVal
	}
	if p == "" {
		p = s.Infra
	}
	if p == "" {
		return "", errors.New(`photo cdk needs contact-sheet's infra/ directory. Clone the repo once:
  git clone https://github.com/PrzemekMalak/contact-sheet.git
then set infra = "<path to contact-sheet>/infra" in [cdk] of contact-sheet.toml,
or pass --infra DIR, or set CONTACT_SHEET_INFRA`)
	}
	if p == "~" || strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		p = filepath.Join(home, p[1:])
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(siteDir, p)
	}
	return filepath.Clean(p), nil
}

// VenvPython is the infra venv's python, relative to the infra directory.
func VenvPython(goos string) string {
	if goos == "windows" {
		return `.venv\Scripts\python.exe`
	}
	return ".venv/bin/python"
}

// Preflight checks that infra looks like contact-sheet's infra/, that its
// venv exists, and that npx is available.
func Preflight(infra, goos string, lookPath func(string) (string, error)) error {
	for _, f := range []string{"cdk.json", "app.py"} {
		if _, err := os.Stat(filepath.Join(infra, f)); err != nil {
			return fmt.Errorf("%s has no %s; is it contact-sheet's infra/ directory?", infra, f)
		}
	}
	py := VenvPython(goos)
	if _, err := os.Stat(filepath.Join(infra, filepath.FromSlash(strings.ReplaceAll(py, `\`, "/")))); err != nil {
		return fmt.Errorf("no Python venv in %s. Create it once:\n  cd %s\n  python3 -m venv .venv\n  %s -m pip install -r requirements.txt",
			infra, infra, py)
	}
	if _, err := lookPath("npx"); err != nil {
		return errors.New("npx not found: install Node.js (the AWS CDK CLI runs through npx)")
	}
	return nil
}

// Args is the npx argument list: the latest CDK CLI, the app run with the
// venv's python, the context as sorted -c flags, then the caller's
// arguments. CDK keeps the last value of a repeated -c key, so a -c typed on
// the command line overrides contact-sheet.toml.
// A --app (or -a) the caller passes replaces ours.
func Args(cdkArgs []string, goos string, ctx map[string]string) []string {
	args := []string{"--yes", "aws-cdk@latest"}
	if !hasApp(cdkArgs) {
		args = append(args, "--app", VenvPython(goos)+" app.py")
	}
	for _, k := range sortedKeys(ctx) {
		args = append(args, "-c", k+"="+ctx[k])
	}
	return append(args, cdkArgs...)
}

func hasApp(args []string) bool {
	for _, a := range args {
		if a == "--app" || a == "-a" || strings.HasPrefix(a, "--app=") || strings.HasPrefix(a, "-a=") {
			return true
		}
	}
	return false
}

// Env adds AWS_PROFILE=profile to environ unless profile is empty or a
// non-empty AWS_PROFILE is already set. An empty AWS_PROFILE= counts as unset.
func Env(environ []string, profile string) []string {
	if profile == "" {
		return environ
	}
	out := make([]string, 0, len(environ)+1)
	for _, kv := range environ {
		if v, ok := strings.CutPrefix(kv, "AWS_PROFILE="); ok {
			if v != "" {
				return environ
			}
			continue
		}
		out = append(out, kv)
	}
	return append(out, "AWS_PROFILE="+profile)
}

// Summary is the line printed before running. It names the context keys
// but not their values, which may be private (an email address).
func Summary(infra, profile string, ctx map[string]string) string {
	if profile == "" {
		profile = "(default credentials)"
	}
	keys := "(none)"
	if len(ctx) > 0 {
		keys = strings.Join(sortedKeys(ctx), ", ")
	}
	return fmt.Sprintf("photo cdk: infra %s, AWS profile %s, context: %s", infra, profile, keys)
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
