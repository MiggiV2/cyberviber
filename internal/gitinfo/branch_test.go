package gitinfo

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestShouldReadBranchFromParentRepo(t *testing.T) {
	// given
	root := t.TempDir()
	write(t, filepath.Join(root, ".git", "HEAD"), "ref: refs/heads/feat/oauth\n")
	sub := filepath.Join(root, "src", "main")
	os.MkdirAll(sub, 0o755)

	// when / then
	if got := Branch(sub); got != "feat/oauth" {
		t.Fatalf("got %q", got)
	}
}

func TestShouldShortenDetachedHead(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, ".git", "HEAD"), "9ed458a1234567890abcdef\n")
	if got := Branch(root); got != "9ed458a" {
		t.Fatalf("got %q", got)
	}
}

func TestShouldFollowWorktreeGitFile(t *testing.T) {
	// given
	root := t.TempDir()
	gitdir := filepath.Join(root, "main", ".git", "worktrees", "wt")
	write(t, filepath.Join(gitdir, "HEAD"), "ref: refs/heads/wt-branch\n")
	write(t, filepath.Join(root, "wt", ".git"), "gitdir: "+gitdir+"\n")

	// when / then
	if got := Branch(filepath.Join(root, "wt")); got != "wt-branch" {
		t.Fatalf("got %q", got)
	}
}

func TestShouldReturnEmptyOutsideRepo(t *testing.T) {
	if got := Branch(t.TempDir()); got != "" {
		t.Fatalf("got %q", got)
	}
}
