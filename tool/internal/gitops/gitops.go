// Package gitops wraps the git CLI for the photo tool's publish flow.
package gitops

import (
	"fmt"
	"os/exec"
)

// run executes `git -C dir args...` and returns stdout. On failure the error
// carries git's combined stdout+stderr verbatim so callers can surface it.
func run(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w\n%s", args[0], err, out)
	}
	return string(out), nil
}

// Status returns the output of `git status --porcelain` for dir.
func Status(dir string) (string, error) {
	return run(dir, "status", "--porcelain")
}

// CommitAll stages every change in dir (`git add -A`) and commits with msg.
func CommitAll(dir, msg string) error {
	if _, err := run(dir, "add", "-A"); err != nil {
		return err
	}
	_, err := run(dir, "commit", "-m", msg)
	return err
}

// Push runs `git push` in dir.
func Push(dir string) error {
	_, err := run(dir, "push")
	return err
}
