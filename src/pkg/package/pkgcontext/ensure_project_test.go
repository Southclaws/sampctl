package pkgcontext

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/lockfile"
	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
	"github.com/Southclaws/sampctl/src/pkg/runtime/run"
)

func TestEnsureProjectInitialisesLockfileAndPinsDependencies(t *testing.T) {
	cacheDir := t.TempDir()
	projectDir := t.TempDir()

	depMeta := versioning.DependencyMeta{User: "testuser", Repo: "testrepo"}
	cachePath := depMeta.CachePath(cacheDir)
	require.NoError(t, os.MkdirAll(cachePath, 0o755))

	repo, err := git.PlainInit(cachePath, false)
	require.NoError(t, err)

	wt, err := repo.Worktree()
	require.NoError(t, err)

	writeDep := func(name, contents string) {
		t.Helper()
		fullPath := filepath.Join(cachePath, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(fullPath), 0o755))
		require.NoError(t, os.WriteFile(fullPath, []byte(contents), 0o644))
		_, err = wt.Add(name)
		require.NoError(t, err)
	}

	commitDep := func(msg string, when time.Time) plumbing.Hash {
		t.Helper()
		hash, err := wt.Commit(msg, &git.CommitOptions{
			Author:    &object.Signature{Name: "test", Email: "test@example.com", When: when},
			Committer: &object.Signature{Name: "test", Email: "test@example.com", When: when},
		})
		require.NoError(t, err)
		return hash
	}

	writeDep("pawn.json", `{"entry":"dep.pwn","output":"gamemodes/dep.amx"}`)
	writeDep("dep.pwn", `main() {}`)
	_ = commitDep("initial", time.Unix(100, 0))
	_, err = repo.CreateTag("1.0.0", headHashForEnsureProject(repo, t), nil)
	require.NoError(t, err)

	rootConfig := map[string]any{
		"entry":        "main.pwn",
		"output":       "gamemodes/main.amx",
		"dependencies": []string{"testuser/testrepo"},
		"runtime": map[string]any{
			"version": "0.3.7",
		},
	}
	configBytes, err := json.MarshalIndent(rootConfig, "", "\t")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "pawn.json"), configBytes, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "main.pwn"), []byte("main() {}"), 0o644))

	seedStagedRuntime(t, cacheDir, run.Runtime{Version: "0.3.7", Platform: "linux"})

	pcx, err := NewPackageContext(nil, nil, true, projectDir, "linux", cacheDir, "", false)
	require.NoError(t, err)
	require.NoError(t, pcx.InitLockfileResolver("dev"))

	updated, err := pcx.EnsureProject(context.Background(), false)
	require.NoError(t, err)
	assert.True(t, updated)

	finalPkg, err := pawnpackage.PackageFromDir(projectDir)
	require.NoError(t, err)
	require.Len(t, finalPkg.Dependencies, 1)
	assert.Equal(t, versioning.DependencyString("testuser/testrepo:1.0.0"), finalPkg.Dependencies[0])

	lf, err := lockfile.Load(projectDir)
	require.NoError(t, err)
	require.NotNil(t, lf)
	assert.True(t, lf.HasRuntime())
	assert.Contains(t, lf.Dependencies, lockfile.DependencyKey(depMeta))
	assert.FileExists(t, filepath.Join(projectDir, lockfile.Filename))
	assert.FileExists(t, filepath.Join(projectDir, "server"))
}

func seedStagedRuntime(t *testing.T, cacheDir string, cfg run.Runtime) {
	t.Helper()

	stageDir := filepath.Join(cacheDir, "runtime_staging", cfg.Platform, cfg.Version)
	require.NoError(t, os.MkdirAll(stageDir, 0o755))

	serverPath := filepath.Join(stageDir, "server")
	serverContents := []byte("runtime-binary")
	require.NoError(t, os.WriteFile(serverPath, serverContents, 0o755))

	hash := sha256.Sum256(serverContents)
	manifest := map[string]any{
		"version":      cfg.Version,
		"platform":     cfg.Platform,
		"runtime_type": string(cfg.GetEffectiveRuntimeType()),
		"files": []map[string]any{{
			"path": "server",
			"size": int64(len(serverContents)),
			"hash": hex.EncodeToString(hash[:]),
			"mode": uint32(0o755),
		}},
	}

	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(stageDir, "sampctl-runtime-manifest.json"), manifestBytes, 0o600))
}

func headHashForEnsureProject(repo *git.Repository, t *testing.T) plumbing.Hash {
	t.Helper()
	head, err := repo.Head()
	require.NoError(t, err)
	return head.Hash()
}
