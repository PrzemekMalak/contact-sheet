package cdk

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"
)

type call struct {
	dir, name string
	args, env []string
}

func fakeExec(code int, got *[]call) Exec {
	return func(dir, name string, args, env []string) (int, error) {
		*got = append(*got, call{dir, name, args, env})
		return code, nil
	}
}

func opts(site string, env []string, ex Exec, log *bytes.Buffer) Options {
	return Options{SiteDir: site, Home: "/home/me", GOOS: "darwin", Environ: env,
		LookPath: okLookPath, Exec: ex, Log: log}
}

func TestRunPassesThrough(t *testing.T) {
	infra := makeInfra(t, "cdk.json", "app.py", ".venv/bin/python")
	site := writeSite(t, "[cdk]\ninfra = \""+infra+"\"\nprofile = \"p\"\n[cdk.context]\nsite_name = \"a\"\n")
	var calls []call
	var log bytes.Buffer
	code, err := Run([]string{"diff"}, opts(site, []string{"PATH=/bin"}, fakeExec(0, &calls), &log))
	if err != nil || code != 0 {
		t.Fatalf("code %d err %v", code, err)
	}
	if len(calls) != 1 {
		t.Fatalf("want 1 exec, got %d", len(calls))
	}
	c := calls[0]
	ctx := map[string]string{"site_name": "a"}
	if c.dir != infra || c.name != "npx" || !reflect.DeepEqual(c.args, Args([]string{"diff"}, "darwin", ctx)) {
		t.Fatalf("unexpected exec %+v", c)
	}
	if !contains(c.env, "AWS_PROFILE=p") {
		t.Fatalf("env lacks AWS_PROFILE=p: %q", c.env)
	}
	if log.String() != Summary(infra, "p", ctx)+"\n" {
		t.Fatalf("log %q", log.String())
	}
}

func TestRunInfraFlag(t *testing.T) {
	infra := makeInfra(t, "cdk.json", "app.py", ".venv/bin/python")
	site := writeSite(t, "[s3]\nbucket = \"\"\n")
	for _, args := range [][]string{{"--infra", infra, "synth"}, {"--infra=" + infra, "synth"}} {
		var calls []call
		code, err := Run(args, opts(site, nil, fakeExec(0, &calls), &bytes.Buffer{}))
		if err != nil || code != 0 || len(calls) != 1 {
			t.Fatalf("%q: code %d err %v calls %d", args, code, err, len(calls))
		}
		if calls[0].dir != infra || !reflect.DeepEqual(calls[0].args, Args([]string{"synth"}, "darwin", nil)) {
			t.Fatalf("%q: unexpected exec %+v", args, calls[0])
		}
		for _, a := range calls[0].args {
			if a == "-c" {
				t.Fatalf("%q: no context expected, got %q", args, calls[0].args)
			}
		}
	}
}

func TestRunEnvInfra(t *testing.T) {
	infra := makeInfra(t, "cdk.json", "app.py", ".venv/bin/python")
	site := writeSite(t, "[cdk]\ninfra = \"/nope\"\n")
	var calls []call
	if _, err := Run([]string{"diff"}, opts(site, []string{"CONTACT_SHEET_INFRA=" + infra}, fakeExec(0, &calls), &bytes.Buffer{})); err != nil {
		t.Fatal(err)
	}
	if calls[0].dir != infra {
		t.Fatalf("env infra not used: %q", calls[0].dir)
	}
}

func TestRunExitCode(t *testing.T) {
	infra := makeInfra(t, "cdk.json", "app.py", ".venv/bin/python")
	site := writeSite(t, "[cdk]\ninfra = \""+infra+"\"\n")
	var calls []call
	code, err := Run([]string{"deploy", "X"}, opts(site, nil, fakeExec(3, &calls), &bytes.Buffer{}))
	if code != 3 || err != nil {
		t.Fatalf("want 3, nil; got %d, %v", code, err)
	}
}

func TestRunUsage(t *testing.T) {
	infra := makeInfra(t, "cdk.json", "app.py", ".venv/bin/python")
	site := writeSite(t, "")
	for _, args := range [][]string{nil, {"--infra", infra}} {
		var calls []call
		code, err := Run(args, opts(site, nil, fakeExec(0, &calls), &bytes.Buffer{}))
		if code != 2 || !errors.Is(err, ErrUsage) || len(calls) != 0 {
			t.Fatalf("%q: got %d, %v, %d calls", args, code, err, len(calls))
		}
	}
	for _, w := range []string{"diff", "deploy", "synth"} {
		if !strings.Contains(ErrUsage.Error(), w) {
			t.Errorf("usage lacks %q", w)
		}
	}
}

func TestRunPreflightError(t *testing.T) {
	infra := makeInfra(t, "cdk.json", "app.py")
	site := writeSite(t, "[cdk]\ninfra = \""+infra+"\"\n")
	var calls []call
	code, err := Run([]string{"diff"}, opts(site, nil, fakeExec(0, &calls), &bytes.Buffer{}))
	if err == nil || code == 0 || len(calls) != 0 {
		t.Fatalf("want preflight error and no exec; got %d, %v, %d calls", code, err, len(calls))
	}
}

func TestRunInfraFlagAnywhere(t *testing.T) {
	infra := makeInfra(t, "cdk.json", "app.py", ".venv/bin/python")
	site := writeSite(t, "[cdk]\ninfra = \"/nope\"\n")
	for _, args := range [][]string{{"diff", "--infra", infra}, {"deploy", "S", "--infra=" + infra}} {
		var calls []call
		if _, err := Run(args, opts(site, nil, fakeExec(0, &calls), &bytes.Buffer{})); err != nil {
			t.Fatalf("%q: %v", args, err)
		}
		if calls[0].dir != infra {
			t.Fatalf("%q: infra %q", args, calls[0].dir)
		}
		for _, a := range calls[0].args {
			if strings.HasPrefix(a, "--infra") || a == infra {
				t.Fatalf("%q: --infra passed on to CDK: %q", args, calls[0].args)
			}
		}
	}
}
