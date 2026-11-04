// Package gitinfo reads the current branch without spawning git.
package gitinfo

import (
	"os"
	"path/filepath"
	"strings"
)

// Branch returns the checked-out branch for dir (or the short commit when detached), or "" outside a repo.
func Branch(dir string) string {
	gitDir := findGitDir(dir)
	if gitDir == "" {
		return ""
	}
	b, err := os.ReadFile(filepath.Join(gitDir, "HEAD"))
	if err != nil {
		return ""
	}
	head := strings.TrimSpace(string(b))
	if ref, ok := strings.CutPrefix(head, "ref: refs/heads/"); ok {
		return ref
	}
	if len(head) >= 7 {
		return head[:7]
	}
	return head
}

func findGitDir(dir string) string {
	for d := filepath.Clean(dir); ; d = filepath.Dir(d) {
		p := filepath.Join(d, ".git")
		if fi, err := os.Stat(p); err == nil {
			if fi.IsDir() {
				return p
			}
			// worktree or submodule: ".git" file pointing to the real git dir
			b, err := os.ReadFile(p)
			if err != nil {
				return ""
			}
			target, ok := strings.CutPrefix(strings.TrimSpace(string(b)), "gitdir: ")
			if !ok {
				return ""
			}
			if !filepath.IsAbs(target) {
				target = filepath.Join(d, target)
			}
			return target
		}
		if d == filepath.Dir(d) {
			return ""
		}
	}
}
