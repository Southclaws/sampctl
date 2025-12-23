package pkgcontext

import (
	"os"
	"path/filepath"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
)

// ValidateRepository performs checks on a git repository to ensure it's in a healthy state
func ValidateRepository(path string) (valid bool, err error) {
	gitDir := filepath.Join(path, ".git")
	if !fs.Exists(gitDir) {
		print.Verb("repository missing .git directory:", path)
		return false, nil
	}

	gitDirInfo, err := os.Stat(gitDir)
	if err != nil {
		print.Verb("cannot stat .git directory:", err)
		return false, nil
	}
	if !gitDirInfo.IsDir() {
		print.Verb(".git is not a directory (possibly a submodule):", path)
		return false, nil
	}

	repo, err := git.PlainOpen(path)
	if err != nil {
		print.Verb("cannot open repository:", err)
		return false, nil
	}

	head, err := repo.Head()
	if err != nil {
		print.Verb("repository has invalid HEAD:", err)
		return false, nil
	}

	_, err = repo.CommitObject(head.Hash())
	if err != nil {
		print.Verb("HEAD points to invalid commit:", err)
		return false, nil
	}

	wt, err := repo.Worktree()
	if err != nil {
		print.Verb("cannot access worktree:", err)
		return false, nil
	}

	_, err = wt.Status()
	if err != nil {
		print.Verb("worktree status check failed:", err)
		return false, nil
	}

	commitIter, err := repo.CommitObjects()
	if err != nil {
		print.Verb("cannot access commits:", err)
		return false, nil
	}
	defer commitIter.Close()

	_, err = commitIter.Next()
	if err != nil {
		print.Verb("repository has no commits (empty):", err)
		return false, nil
	}

	objectsDir := filepath.Join(gitDir, "objects")
	if !fs.Exists(objectsDir) {
		print.Verb("objects directory missing:", path)
		return false, nil
	}

	refsDir := filepath.Join(gitDir, "refs")
	if !fs.Exists(refsDir) {
		print.Verb("refs directory missing:", path)
		return false, nil
	}

	print.Verb("repository validated successfully:", path)
	return true, nil
}

// ValidateRepositoryWithRefs performs validation and also checks if specific refs exist
func ValidateRepositoryWithRefs(path string, requiredRefs []string) (valid bool, err error) {
	valid, err = ValidateRepository(path)
	if err != nil || !valid {
		return valid, err
	}

	if len(requiredRefs) == 0 {
		return true, nil
	}

	repo, err := git.PlainOpen(path)
	if err != nil {
		return false, err
	}

	for _, refName := range requiredRefs {
		_, err := repo.Reference(plumbing.ReferenceName(refName), true)
		if err != nil {
			print.Verb("required reference not found:", refName)
			return false, nil
		}
	}

	return true, nil
}

// CleanInvalidRepository removes a repository if it fails validation
func CleanInvalidRepository(path string) error {
	if !fs.Exists(path) {
		return nil
	}

	valid, err := ValidateRepository(path)
	if err != nil {
		print.Verb("validation error, removing repository:", path, err)
		return os.RemoveAll(path)
	}

	if !valid {
		print.Verb("removing invalid repository:", path)
		return os.RemoveAll(path)
	}

	print.Verb("repository is valid, keeping:", path)
	return nil
}

// DiagnoseRepository provides detailed diagnostic information about a repository
func DiagnoseRepository(path string) (diagnosis string, healthy bool) {
	gitDir := filepath.Join(path, ".git")

	if !fs.Exists(gitDir) {
		return "Missing .git directory", false
	}

	repo, err := git.PlainOpen(path)
	if err != nil {
		return "Cannot open as git repository: " + err.Error(), false
	}

	head, err := repo.Head()
	if err != nil {
		return "Invalid or missing HEAD: " + err.Error(), false
	}

	commit, err := repo.CommitObject(head.Hash())
	if err != nil {
		return "HEAD points to non-existent commit: " + err.Error(), false
	}

	wt, err := repo.Worktree()
	if err != nil {
		return "Cannot access worktree: " + err.Error(), false
	}

	status, err := wt.Status()
	if err != nil {
		return "Cannot get worktree status: " + err.Error(), false
	}

	commitCount := 0
	commitIter, err := repo.CommitObjects()
	if err == nil {
		defer commitIter.Close()
		_ = commitIter.ForEach(func(c *object.Commit) error {
			commitCount++
			return nil
		})
	}

	refIter, err := repo.References()
	refCount := 0
	if err == nil {
		defer refIter.Close()
		_ = refIter.ForEach(func(ref *plumbing.Reference) error {
			refCount++
			return nil
		})
	}

	dirtyFiles := 0
	for _, fileStatus := range status {
		if fileStatus.Worktree != git.Unmodified || fileStatus.Staging != git.Unmodified {
			dirtyFiles++
		}
	}

	diagnosis = "Healthy repository"
	if dirtyFiles > 0 {
		diagnosis += " (with uncommitted changes)"
	}

	return diagnosis + " | HEAD: " + head.Hash().String()[:8] + " | " + commit.Message[:min(50, len(commit.Message))] +
		" | Commits: " + string(rune(commitCount)) + " | Refs: " + string(rune(refCount)), true
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// RepairRepository attempts to repair a corrupted repository
func RepairRepository(path string) error {
	print.Verb("attempting to repair repository:", path)

	repo, err := git.PlainOpen(path)
	if err != nil {
		return errors.Wrap(err, "cannot open repository for repair")
	}

	wt, err := repo.Worktree()
	if err != nil {
		return errors.Wrap(err, "cannot access worktree for repair")
	}

	err = wt.Clean(&git.CleanOptions{
		Dir: true,
	})
	if err != nil {
		print.Verb("failed to clean worktree:", err)
	}

	head, err := repo.Head()
	if err != nil {
		return errors.Wrap(err, "cannot get HEAD for repair")
	}

	err = wt.Reset(&git.ResetOptions{
		Commit: head.Hash(),
		Mode:   git.HardReset,
	})
	if err != nil {
		return errors.Wrap(err, "failed to reset worktree")
	}

	print.Verb("repository repair completed:", path)
	return nil
}
