//go:build linux || darwin

package rook

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
)

func TestReleaseRejectsDirtyWorktree(t *testing.T) {
	root, _ := setupTestRepository(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, "uncommitted.txt"), []byte("uncommitted"), 0o644))

	runTestWizard(t, "TestReleaseDirtyWorktreeProcess", []wizardAnswer{
		{"Choose the first version", "\r"},
		{"Write", "\r"},
		{"Create a GitHub release?", "\r"},
	}, "SAMPCTL_DIRTY_RELEASE_DIR="+root)
}

func TestReleaseDirtyWorktreeProcess(t *testing.T) {
	root := os.Getenv("SAMPCTL_DIRTY_RELEASE_DIR")
	if root == "" {
		return
	}

	err := Release(t.Context(), nil, nil, pawnpackage.Package{LocalPath: root, Repo: "fixture"})
	require.ErrorContains(t, err, "working directory has uncommitted changes")
}

func TestReleaseRejectsMissingGitHubMetadata(t *testing.T) {
	for _, tc := range []struct {
		name string
		user string
		repo string
	}{
		{name: "missing owner", repo: "fixture"},
		{name: "missing repository", user: "fixture"},
		{name: "missing both"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, repo := setupTestRepository(t)
			remote := t.TempDir()
			remoteRepo, err := git.PlainInit(remote, true)
			require.NoError(t, err)
			_, err = repo.CreateRemote(&config.RemoteConfig{Name: "origin", URLs: []string{remote}})
			require.NoError(t, err)

			runTestWizard(t, "TestReleaseMissingGitHubMetadataProcess", []wizardAnswer{
				{"Choose the first version", "\r"},
				{"Write", "\r"},
				{"Create a GitHub release?", "y\r"},
			}, "SAMPCTL_RELEASE_FIXTURE="+root, "SAMPCTL_RELEASE_USER="+tc.user, "SAMPCTL_RELEASE_REPO="+tc.repo)

			head, err := repo.Head()
			require.NoError(t, err)
			pushedBranch, err := remoteRepo.Reference(head.Name(), true)
			require.NoError(t, err)
			pushedTag, err := remoteRepo.Reference(plumbing.NewTagReferenceName("0.0.1"), true)
			require.NoError(t, err)
			require.Equal(t, head.Hash(), pushedBranch.Hash())
			require.Equal(t, head.Hash(), pushedTag.Hash())
		})
	}
}

func TestReleaseMissingGitHubMetadataProcess(t *testing.T) {
	root := os.Getenv("SAMPCTL_RELEASE_FIXTURE")
	if root == "" {
		return
	}
	pkg := pawnpackage.Package{
		LocalPath: root,
		User:      os.Getenv("SAMPCTL_RELEASE_USER"),
		Repo:      os.Getenv("SAMPCTL_RELEASE_REPO"),
	}

	err := Release(t.Context(), nil, nil, pkg)

	require.ErrorContains(t, err, "set `user` and `repo` in the package definition")
}
