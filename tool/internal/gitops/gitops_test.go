package gitops

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// initRepo creates a hermetic git repo in a temp dir with a local identity,
// so commits work without any global git configuration.
func initRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "Test"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return dir
}

func writeFile(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestStatusListsUntracked(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "new.txt")

	out, err := Status(dir)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !strings.Contains(out, "?? new.txt") {
		t.Errorf("Status output %q does not list untracked new.txt", out)
	}
}

func TestCommitAllCreatesCommit(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "new.txt")

	if err := CommitAll(dir, "test commit"); err != nil {
		t.Fatalf("CommitAll: %v", err)
	}

	log, err := exec.Command("git", "-C", dir, "log", "--oneline").CombinedOutput()
	if err != nil {
		t.Fatalf("git log: %v\n%s", err, log)
	}
	lines := strings.Split(strings.TrimSpace(string(log)), "\n")
	if len(lines) != 1 || !strings.Contains(lines[0], "test commit") {
		t.Errorf("git log = %q, want exactly one commit 'test commit'", log)
	}

	status, err := Status(dir)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if status != "" {
		t.Errorf("Status after commit = %q, want clean", status)
	}
}
