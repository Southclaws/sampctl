package lockfile

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
	"github.com/go-git/go-git/v5/plumbing/format/index"
	"github.com/go-git/go-git/v5/utils/merkletrie"
	"github.com/go-git/go-git/v5/utils/merkletrie/filesystem"
	mindex "github.com/go-git/go-git/v5/utils/merkletrie/index"
	"github.com/go-git/go-git/v5/utils/merkletrie/noder"
)

func cleanWorktreeWithObsoleteSubmodules(ctx context.Context, repo *git.Repository, worktree *git.Worktree, status git.Status) (bool, error) {
	if status.IsClean() {
		return true, nil
	}
	for _, file := range status {
		if file.Staging != git.Unmodified && file.Staging != git.Untracked {
			return false, nil
		}
	}
	idx, err := repo.Storer.Index()
	if err != nil {
		return false, err
	}
	modules, err := worktree.Submodules()
	if err != nil {
		return false, err
	}
	actualModules := make(map[string]plumbing.Hash)
	obsolete := false
	for _, module := range modules {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		path := module.Config().Path
		entry, err := idx.Entry(path)
		if err != nil && err != index.ErrEntryNotFound {
			return false, err
		}
		if entry == nil || entry.Mode != filemode.Submodule {
			obsolete = true
			continue
		}
		moduleStatus, err := module.Status()
		if err != nil {
			return false, err
		}
		hash := moduleStatus.Current
		if hash.IsZero() {
			hash = moduleStatus.Expected
		}
		actualModules[path] = hash
	}
	if !obsolete {
		return false, nil
	}

	// GoGit treats every .gitmodules path as a gitlink, even after its files
	// are vendored. Compare those directories normally while retaining gitlinks.
	changes, err := merkletrie.DiffTreeContext(ctx, mindex.NewRootNode(idx), filesystem.NewRootNode(worktree.Filesystem, actualModules), equalIntegrityNodes)
	if err != nil {
		return false, fmt.Errorf("compare vendored worktree files: %w", err)
	}
	patterns, err := gitignore.ReadPatterns(worktree.Filesystem, nil)
	if err != nil {
		return false, err
	}
	matcher := gitignore.NewMatcher(append(patterns, worktree.Excludes...))
	for _, change := range changes {
		if len(change.From) == 0 && matcher.Match(strings.Split(change.To.String(), "/"), change.To.IsDir()) {
			continue
		}
		return false, nil
	}
	return true, ctx.Err()
}

func equalIntegrityNodes(a, b noder.Hasher) bool {
	hashA, hashB := a.Hash(), b.Hash()
	// GoGit directory hashes are zero and must be compared by their contents.
	var empty [24]byte
	if bytes.Equal(hashA, empty[:]) || bytes.Equal(hashB, empty[:]) {
		return false
	}
	return bytes.Equal(hashA, hashB)
}
