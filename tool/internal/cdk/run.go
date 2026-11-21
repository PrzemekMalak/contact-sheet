package cdk

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// ErrUsage is returned when photo cdk gets no CDK arguments.
var ErrUsage = errors.New(`usage: photo cdk [--infra DIR] <cdk arguments...>

Runs the AWS CDK CLI in contact-sheet's infra/ with the [cdk] settings from
contact-sheet.toml. Common commands:
  photo cdk diff                      what would change
  photo cdk deploy PhotoSiteStack     deploy one stack (use your stack_prefix)
  photo cdk synth                     print the CloudFormation templates`)

// Exec runs name with args in dir. err is set only when the process could
// not be started; otherwise exitCode is its exit status.
type Exec func(dir, name string, args, env []string) (exitCode int, err error)

// Options are Run's dependencies, injected so tests don't run real CDK.
type Options struct {
	SiteDir  string
	Home     string
	GOOS     string
	Environ  []string
	LookPath func(string) (string, error)
	Exec     Exec
	Log      io.Writer
}

// Run handles `photo cdk [--infra DIR] <args...>` and returns the exit code
// to use. A non-nil error means CDK didn't run.
func Run(args []string, o Options) (int, error) {
	// --infra DIR is ours wherever it appears; everything else goes to CDK.
	var flagInfra string
	var rest []string
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == "--infra" && i+1 < len(args):
			flagInfra = args[i+1]
			i++
		case strings.HasPrefix(a, "--infra="):
			flagInfra = strings.TrimPrefix(a, "--infra=")
		default:
			rest = append(rest, a)
		}
	}
	args = rest
	if len(args) == 0 {
		return 2, ErrUsage
	}
	s, err := LoadSettings(o.SiteDir)
	if err != nil {
		return 1, err
	}
	infra, err := ResolveInfra(flagInfra, getenv(o.Environ, "CONTACT_SHEET_INFRA"), o.SiteDir, o.Home, s)
	if err != nil {
		return 1, err
	}
	if err := Preflight(infra, o.GOOS, o.LookPath); err != nil {
		return 1, err
	}
	env := Env(o.Environ, s.Profile)
	fmt.Fprintln(o.Log, Summary(infra, getenv(env, "AWS_PROFILE"), s.Context))
	code, err := o.Exec(infra, "npx", Args(args, o.GOOS, s.Context), env)
	if err != nil {
		return 1, fmt.Errorf("run npx: %w", err)
	}
	return code, nil
}

// OSExec runs the command with the terminal attached.
func OSExec(dir, name string, args, env []string) (int, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir, cmd.Env = dir, env
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	err := cmd.Run()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), nil
	}
	if err != nil {
		return 1, err
	}
	return 0, nil
}

func getenv(environ []string, key string) string {
	for i := len(environ) - 1; i >= 0; i-- {
		if v, ok := strings.CutPrefix(environ[i], key+"="); ok {
			return v
		}
	}
	return ""
}
