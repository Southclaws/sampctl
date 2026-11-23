package pkgcontext

import (
	"context"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

func readBuildGitInfo(ctx context.Context, repoPath string) (commit, branch string) {
	gitDir := filepath.Join(repoPath, ".git")
	if _, err := os.Stat(gitDir); err != nil {
		return "", ""
	}
	git, err := exec.LookPath("git")
	if errors.Is(err, exec.ErrNotFound) {
		return readLooseBuildGitInfo(gitDir)
	}
	if err != nil {
		return "", ""
	}

	runGit := func(args ...string) string {
		cmd := exec.CommandContext(ctx, git, args...)
		cmd.Dir = repoPath
		cmd.Env = slices.DeleteFunc(cmd.Environ(), func(entry string) bool {
			return strings.HasPrefix(strings.ToUpper(entry), "GIT_")
		})
		output, err := cmd.Output()
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(output))
	}

	commit = runGit("rev-parse", "--verify", "HEAD^{commit}")
	if !isBuildCommitID(commit) {
		return "", ""
	}
	ref := runGit("symbolic-ref", "--quiet", "HEAD")
	if strings.HasPrefix(ref, "refs/heads/") {
		branch = path.Base(ref)
	}
	return commit, branch
}

func readLooseBuildGitInfo(gitDir string) (commit, branch string) {
	data, err := os.ReadFile(filepath.Join(gitDir, "HEAD"))
	if err != nil {
		return "", ""
	}
	head := strings.TrimSpace(string(data))
	if isBuildCommitID(head) {
		return head, ""
	}

	ref, symbolic := strings.CutPrefix(head, "ref: ")
	if !symbolic || !strings.HasPrefix(ref, "refs/heads/") ||
		ref == "refs/heads/" || strings.Contains(ref, "\\") || !filepath.IsLocal(filepath.FromSlash(ref)) {
		return "", ""
	}
	for name := range strings.SplitSeq(ref, "/") {
		if name == "" || name == "." || name == ".." {
			return "", ""
		}
	}
	data, err = os.ReadFile(filepath.Join(gitDir, filepath.FromSlash(ref)))
	if err != nil {
		return "", ""
	}
	commit = strings.TrimSpace(string(data))
	if !isBuildCommitID(commit) {
		return "", ""
	}
	return commit, path.Base(ref)
}

func isBuildCommitID(value string) bool {
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
