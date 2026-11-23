package pkgcontext

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/kirsle/configdir"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/lockfile"
	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
	"github.com/Southclaws/sampctl/src/pkg/runtime"
	runtimecfg "github.com/Southclaws/sampctl/src/pkg/runtime/config"
)

type overrideFailingProvisioner struct {
	fakeRuntimeProvisioner
	beforeFailure func()
	err           error
}

func (p *overrideFailingProvisioner) EnsureBinaries(context.Context, string, runtimecfg.Runtime) (*runtime.RuntimeManifestInfo, error) {
	p.beforeFailure()
	return nil, p.err
}

func useOfflineProjectOverrides(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Cleanup(configdir.Refresh)
	for _, name := range []string{"HOME", "XDG_CONFIG_HOME", "APPDATA"} {
		t.Setenv(name, dir)
	}
	configdir.Refresh()
	versioning.ResetDependencyOverrides()
	t.Cleanup(versioning.ResetDependencyOverrides)

	path := versioning.DefaultDependencyOverridesCachePath()
	relative, err := filepath.Rel(dir, path)
	require.NoError(t, err)
	require.True(t, filepath.IsLocal(relative))
	require.NoError(t, os.WriteFile(path, []byte(`{"overrides":{}}`), 0o600))
}

func TestProjectOverridesEnsureGraph(t *testing.T) {
	useOfflineProjectOverrides(t)

	const old = "github.com/IllidanS4/PawnPlus:v1.0.0-pre4"
	const dev = "github.com/IllidanS4/PawnPlus:v1.4.0.1"
	const pinned = "github.com/IllidanS4/PawnPlus:v1.5.2"
	const library = "fixture/library:1.0.0"

	root := lifecycleContext(t, "json")
	leafMeta := graphRevisionMeta(t, pinned)
	libraryMeta := graphRevisionMeta(t, library)
	libraryKey := lockfile.DependencyKey(libraryMeta)
	leafKey := lockfile.DependencyKey(leafMeta)
	seedEnsureProjectDependencyRepo(t, root.CacheDir, leafMeta, []string{"v1.0.0-pre4", "v1.4.0.1", "v1.5.2", "v2.0.0"})
	libraryPath := libraryMeta.CachePath(root.CacheDir)
	seedReliabilityRepo(t, libraryPath, `{"dependencies":["`+old+`"],"overrides":{"`+old+`":"`+dev+`"}}`)

	root.Package.Dependencies = []versioning.DependencyString{library, pinned}
	root.Package.Development = []versioning.DependencyString{dev}
	root.Package.Overrides = map[string]string{old: pinned, dev: pinned}
	require.NoError(t, root.Package.WriteDefinition())
	require.NoError(t, root.InitLockfileResolver("test"))

	manifestPath := filepath.Join(root.Package.LocalPath, "pawn.json")
	lockPath := filepath.Join(root.Package.LocalPath, lockfile.Filename)
	installedPath := filepath.Join(root.Package.LocalPath, "dependencies", "PawnPlus", "dep.pwn")
	read := func(path string) string {
		t.Helper()
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		return string(data)
	}
	originalManifest := read(manifestPath)
	for _, phase := range []string{"initial ensure", "repeat ensure with lockfile"} {
		updated, err := root.EnsureProject(t.Context(), DependencyUpdateRequest{})
		require.NoError(t, err, phase)

		assert.False(t, updated, phase)
		assert.Equal(t, []versioning.DependencyMeta{libraryMeta, leafMeta}, root.AllDependencies, phase)
		assert.Contains(t, read(installedPath), "v1.5.2", phase)
		assert.Equal(t, originalManifest, read(manifestPath), phase)
		entry := root.GetLockfile().Dependencies[leafKey]
		assert.Equal(t, "v1.5.2", entry.Resolved, phase)
		assert.False(t, entry.Transitive, phase)
		assert.Equal(t, []string{libraryKey}, entry.RequiredBy, phase)
	}
	locked := read(lockPath)
	// A direct occurrence visited first must still respect the active transitive pin.
	root.Package.Dependencies[0], root.Package.Dependencies[1] = root.Package.Dependencies[1], root.Package.Dependencies[0]
	require.NoError(t, root.Package.WriteDefinition())
	require.NoError(t, root.UpdateProjectLockfile(t.Context(), DependencyUpdateRequest{Enabled: true, Force: true}))
	assert.Equal(t, locked, read(lockPath))
	root.Package.Dependencies[0], root.Package.Dependencies[1] = root.Package.Dependencies[1], root.Package.Dependencies[0]
	require.NoError(t, root.Package.WriteDefinition())

	fromLibrary, err := LoadPackageContext(NewPackageContextOptions{
		Dir: libraryPath, CacheDir: root.CacheDir, Parent: true,
	})
	require.NoError(t, err)
	require.NoError(t, fromLibrary.EnsureDependenciesCachedContext(t.Context()))
	assert.Equal(t, []versioning.DependencyMeta{graphRevisionMeta(t, dev)}, fromLibrary.AllDependencies)

	other := lifecycleContext(t, "yaml")
	other.CacheDir = root.CacheDir
	other.Package.Dependencies = []versioning.DependencyString{library}
	require.NoError(t, other.EnsureDependenciesCachedContext(t.Context()))
	assert.Equal(t, []versioning.DependencyMeta{libraryMeta, graphRevisionMeta(t, old)}, other.AllDependencies)

	delete(root.Package.Overrides, old)
	require.NoError(t, root.Package.WriteDefinition())
	_, err = root.EnsureProject(t.Context(), DependencyUpdateRequest{})
	var conflict *DependencyDestinationConflictError
	require.ErrorAs(t, err, &conflict)
	assert.Contains(t, err.Error(), "v1.0.0-pre4")
	assert.Contains(t, err.Error(), "v1.5.2")
	assert.Equal(t, locked, read(lockPath))
	assert.Contains(t, read(installedPath), "v1.5.2")

	root.Package.Overrides["unused/invalid"] = "invalid replacement"
	require.NoError(t, root.Package.WriteDefinition())
	_, err = root.EnsureProject(t.Context(), DependencyUpdateRequest{})
	require.ErrorContains(t, err, `pawn.json overrides: invalid replacement "invalid replacement" for "unused/invalid"`)
	assert.Equal(t, locked, read(lockPath))
	assert.Contains(t, read(installedPath), "v1.5.2")

	const newer = "github.com/IllidanS4/PawnPlus:v2.0.0"
	root.Package.Overrides = map[string]string{old: newer, dev: newer, pinned: newer}
	require.NoError(t, root.Package.WriteDefinition())
	newManifest := read(manifestPath)
	sentinel := errors.New("runtime failed after installing replacement")
	root.RuntimeProv = &overrideFailingProvisioner{
		beforeFailure: func() { assert.Contains(t, read(installedPath), "v2.0.0") },
		err:           sentinel,
	}
	_, err = root.EnsureProject(t.Context(), DependencyUpdateRequest{})
	require.ErrorIs(t, err, sentinel)
	assert.Equal(t, newManifest, read(manifestPath))
	assert.Equal(t, locked, read(lockPath))
	assert.Contains(t, read(installedPath), "v1.5.2")
	assertNoDependencyStages(t, root.Package.Vendor)
}

func TestProjectOverridesPreservedByDependencyUpdate(t *testing.T) {
	useOfflineProjectOverrides(t)

	for _, format := range []string{"json", "yaml"} {
		t.Run(format, func(t *testing.T) {
			pcx := lifecycleContext(t, format)
			pcx.Package.Dependencies = []versioning.DependencyString{"fixture/dep:1.0.0", "fixture/legacy:1.0.0"}
			pcx.Package.Overrides = map[string]string{"fixture/legacy:1.0.0": "fixture/replacement:1.0.0"}
			require.NoError(t, pcx.Package.WriteDefinition())
			seedEnsureProjectDependencyRepo(t, pcx.CacheDir, graphRevisionMeta(t, "fixture/dep"), []string{"1.0.0", "2.0.0"})
			seedEnsureProjectDependencyRepo(t, pcx.CacheDir, graphRevisionMeta(t, "fixture/replacement"), []string{"1.0.0", "2.0.0"})
			updated, err := pcx.EnsureProject(t.Context(), DependencyUpdateRequest{Enabled: true, Force: true})
			require.NoError(t, err)
			assert.True(t, updated)
			written, err := pawnpackage.PackageFromDir(pcx.Package.LocalPath)
			require.NoError(t, err)
			assert.Equal(t, pcx.Package.Overrides, written.Overrides)
			assert.Equal(t, []versioning.DependencyString{"fixture/dep:2.0.0", "fixture/legacy:1.0.0"}, written.Dependencies)
			assert.Contains(t, pcx.AllDependencies, graphRevisionMeta(t, "fixture/replacement:1.0.0"))
		})
	}
}

func TestProjectOverridesUnusedRuleDoesNotPinForcedUpdate(t *testing.T) {
	useOfflineProjectOverrides(t)
	for _, lockOnly := range []bool{false, true} {
		name := "ensure"
		if lockOnly {
			name = "lock-only"
		}
		t.Run(name, func(t *testing.T) {
			pcx := lifecycleContext(t, "json")
			meta := graphRevisionMeta(t, "fixture/current:1.0.0")
			seedEnsureProjectDependencyRepo(t, pcx.CacheDir, meta, []string{"1.0.0", "2.0.0"})
			pcx.Package.Dependencies = []versioning.DependencyString{"fixture/current:1.0.0"}
			pcx.Package.Overrides = map[string]string{"fixture/unused:1.0.0": "fixture/current:1.0.0"}
			require.NoError(t, pcx.Package.WriteDefinition())
			require.NoError(t, pcx.InitLockfileResolver("test"))
			request := DependencyUpdateRequest{Enabled: true, Force: true}
			if lockOnly {
				require.NoError(t, pcx.UpdateProjectLockfile(t.Context(), request))
			} else {
				updated, err := pcx.EnsureProject(t.Context(), request)
				require.NoError(t, err)
				assert.True(t, updated)
			}
			assert.Equal(t, "2.0.0", pcx.GetLockfile().Dependencies[lockfile.DependencyKey(meta)].Resolved)
		})
	}
}

func TestProjectOverridesSelectsOriginalRepository(t *testing.T) {
	useOfflineProjectOverrides(t)
	pcx := lifecycleContext(t, "json")
	meta := graphRevisionMeta(t, "fixture/current:1.0.0")
	seedEnsureProjectDependencyRepo(t, pcx.CacheDir, meta, []string{"1.0.0", "2.0.0"})
	pcx.Package.Dependencies = []versioning.DependencyString{"Southclaws/samp-stdlib:1.0.0"}
	pcx.Package.Overrides = map[string]string{"Southclaws/samp-stdlib:1.0.0": "fixture/current:1.0.0"}
	require.NoError(t, pcx.Package.WriteDefinition())
	require.NoError(t, pcx.InitLockfileResolver("test"))
	updated, err := pcx.EnsureProject(t.Context(), DependencyUpdateRequest{
		Enabled: true, Force: true, Target: "Southclaws/samp-stdlib", TargetMeta: graphRevisionMeta(t, "Southclaws/samp-stdlib"),
	})
	require.NoError(t, err)
	assert.False(t, updated)
	assert.Equal(t, "1.0.0", pcx.GetLockfile().Dependencies[lockfile.DependencyKey(meta)].Resolved)
}

func TestProjectOverridesIgnoresMalformedDownloadedRules(t *testing.T) {
	useOfflineProjectOverrides(t)
	pcx := lifecycleContext(t, "json")
	meta := graphRevisionMeta(t, "fixture/child:1.0.0")
	seedReliabilityRepo(t, meta.CachePath(pcx.CacheDir), `{"overrides":{"fixture/old":42}}`)
	pcx.Package.Dependencies = []versioning.DependencyString{"fixture/child:1.0.0"}
	require.NoError(t, pcx.Package.WriteDefinition())
	require.NoError(t, pcx.InitLockfileResolver("test"))
	_, err := pcx.EnsureProject(t.Context(), DependencyUpdateRequest{})
	require.NoError(t, err)
	assert.Equal(t, []versioning.DependencyMeta{meta}, pcx.AllDependencies)
	_, err = pawnpackage.PackageFromDir(meta.CachePath(pcx.CacheDir))
	require.ErrorContains(t, err, "invalid replacement")
}

func TestProjectOverridesLockOnlyFindsPinsIntroducedByUpgrade(t *testing.T) {
	useOfflineProjectOverrides(t)
	pcx := lifecycleContext(t, "json")
	pawnPlus := graphRevisionMeta(t, "fixture/PawnPlus:1.5.2")
	library := graphRevisionMeta(t, "fixture/library:1.0.0")
	seedEnsureProjectDependencyRepo(t, pcx.CacheDir, pawnPlus, []string{"1.5.2", "2.0.0"})
	libraryPath := library.CachePath(pcx.CacheDir)
	repo := seedReliabilityRepo(t, libraryPath, `{}`)
	pcx.Package.Dependencies = []versioning.DependencyString{"fixture/PawnPlus:1.5.2", "fixture/library:1.0.0"}
	pcx.Package.Overrides = map[string]string{"fixture/PawnPlus:1.0.0": "fixture/PawnPlus:1.5.2"}
	require.NoError(t, pcx.Package.WriteDefinition())
	require.NoError(t, pcx.InitLockfileResolver("test"))
	_, err := pcx.EnsureProject(t.Context(), DependencyUpdateRequest{})
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(libraryPath, "pawn.json"), []byte(`{"dependencies":["fixture/PawnPlus:1.0.0"]}`), 0o644))
	worktree, err := repo.Worktree()
	require.NoError(t, err)
	_, err = worktree.Add("pawn.json")
	require.NoError(t, err)
	revision, err := worktree.Commit("request PawnPlus", &git.CommitOptions{Author: &object.Signature{Name: "fixture", Email: "fixture@example.com"}})
	require.NoError(t, err)
	_, err = repo.CreateTag("2.0.0", revision, nil)
	require.NoError(t, err)

	require.NoError(t, pcx.UpdateProjectLockfile(t.Context(), DependencyUpdateRequest{Enabled: true, Force: true}))
	deps := pcx.GetLockfile().Dependencies
	assert.Equal(t, "2.0.0", deps[lockfile.DependencyKey(library)].Resolved)
	assert.Equal(t, "1.5.2", deps[lockfile.DependencyKey(pawnPlus)].Resolved)
	assert.Equal(t, []string{lockfile.DependencyKey(library)}, deps[lockfile.DependencyKey(pawnPlus)].RequiredBy)

	_, err = pcx.EnsureProject(t.Context(), DependencyUpdateRequest{})
	require.NoError(t, err)
	contents, err := os.ReadFile(filepath.Join(pcx.Package.Vendor, "PawnPlus", "dep.pwn"))
	require.NoError(t, err)
	assert.Contains(t, string(contents), "1.5.2")
}

func TestProjectOverridesUnusedRuleDoesNotBlockConflictRepair(t *testing.T) {
	useOfflineProjectOverrides(t)
	pcx := lifecycleContext(t, "json")
	pawnPlus := graphRevisionMeta(t, "fixture/PawnPlus:1.5.2")
	library := graphRevisionMeta(t, "fixture/library:1.0.0")
	seedEnsureProjectDependencyRepo(t, pcx.CacheDir, pawnPlus, []string{"1.5.2", "2.0.0"})
	seedReliabilityRepo(t, library.CachePath(pcx.CacheDir), `{"dependencies":["fixture/PawnPlus:2.0.0"]}`)
	pcx.Package.Dependencies = []versioning.DependencyString{"fixture/PawnPlus:1.5.2", "fixture/library:1.0.0"}
	pcx.Package.Overrides = map[string]string{"fixture/unused:1.0.0": "fixture/PawnPlus:1.5.2"}
	require.NoError(t, pcx.Package.WriteDefinition())
	require.NoError(t, pcx.InitLockfileResolver("test"))

	updated, err := pcx.EnsureProject(t.Context(), DependencyUpdateRequest{Enabled: true, Force: true})
	require.NoError(t, err)
	assert.True(t, updated)
	assert.Equal(t, "2.0.0", pcx.GetLockfile().Dependencies[lockfile.DependencyKey(pawnPlus)].Resolved)
}

func TestProjectOverridesEnsureRepairsStaleLock(t *testing.T) {
	useOfflineProjectOverrides(t)
	pcx, pawnPlus := projectWithStalePawnPlusLock(t)
	declareLibraryRequestingOldPawnPlus(t, pcx)

	_, err := pcx.EnsureProject(t.Context(), DependencyUpdateRequest{})

	require.NoError(t, err)
	assert.Contains(t, installedPawnPlusSource(t, pcx), "v1.5.2")
	_, stale := pcx.GetLockfile().ConflictingOverrideTag(pawnPlus)
	assert.False(t, stale)
}

func TestProjectOverridesLockOnlyRepairsStaleLock(t *testing.T) {
	useOfflineProjectOverrides(t)
	pcx, pawnPlus := projectWithStalePawnPlusLock(t)
	declareLibraryRequestingOldPawnPlus(t, pcx)

	require.NoError(t, pcx.UpdateProjectLockfile(t.Context(), DependencyUpdateRequest{}))

	_, stale := pcx.GetLockfile().ConflictingOverrideTag(pawnPlus)
	require.False(t, stale)

	_, err := pcx.EnsureProject(t.Context(), DependencyUpdateRequest{})

	require.NoError(t, err)
	assert.Contains(t, installedPawnPlusSource(t, pcx), "v1.5.2")
	_, stale = pcx.GetLockfile().ConflictingOverrideTag(pawnPlus)
	assert.False(t, stale)
}

func TestProjectOverridesUnusedRulePreservesExistingLock(t *testing.T) {
	useOfflineProjectOverrides(t)
	pcx, pawnPlus := projectWithStalePawnPlusLock(t)

	_, err := pcx.EnsureProject(t.Context(), DependencyUpdateRequest{})

	require.NoError(t, err)
	assert.Contains(t, installedPawnPlusSource(t, pcx), "2.0.0")
	assert.Equal(t, "2.0.0", pcx.GetLockfile().Dependencies[lockfile.DependencyKey(pawnPlus)].Resolved)
}

func projectWithStalePawnPlusLock(t *testing.T) (*PackageContext, versioning.DependencyMeta) {
	t.Helper()

	const pinned = "fixture/PawnPlus:1.5.2"
	pcx := lifecycleContext(t, "json")
	pawnPlus := graphRevisionMeta(t, pinned)
	seedEnsureProjectDependencyRepo(t, pcx.CacheDir, pawnPlus, []string{"v1.5.2", "2.0.0"})
	pcx.Package.Dependencies = []versioning.DependencyString{pinned}
	require.NoError(t, pcx.Package.WriteDefinition())
	require.NoError(t, pcx.InitLockfileResolver("test"))
	require.NoError(t, pcx.UpdateProjectLockfile(t.Context(), DependencyUpdateRequest{Enabled: true, Force: true}))
	require.Equal(t, "2.0.0", pcx.GetLockfile().Dependencies[lockfile.DependencyKey(pawnPlus)].Resolved)

	pcx.Package.Overrides = map[string]string{"fixture/PawnPlus:1.0.0": pinned}
	require.NoError(t, pcx.Package.WriteDefinition())
	return pcx, pawnPlus
}

func declareLibraryRequestingOldPawnPlus(t *testing.T, pcx *PackageContext) {
	t.Helper()

	const declaration = "fixture/library:1.0.0"
	library := graphRevisionMeta(t, declaration)
	seedReliabilityRepo(t, library.CachePath(pcx.CacheDir), `{"dependencies":["fixture/PawnPlus:1.0.0"]}`)
	pcx.Package.Dependencies = append(pcx.Package.Dependencies, declaration)
	require.NoError(t, pcx.Package.WriteDefinition())
}

func installedPawnPlusSource(t *testing.T, pcx *PackageContext) string {
	t.Helper()

	source, err := os.ReadFile(filepath.Join(pcx.Package.Vendor, "PawnPlus", "dep.pwn"))
	require.NoError(t, err)
	return string(source)
}
