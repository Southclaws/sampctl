package commands

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/lockfile"
)

func TestCheckLockfile(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    checkStatus
	}{
		{name: "optional lockfile missing", want: checkSkipped},
		{name: "malformed", content: "{", want: checkFailed},
		{name: "missing version", content: `{"dependencies":{}}`, want: checkFailed},
		{name: "negative version", content: `{"version":-1,"dependencies":{}}`, want: checkFailed},
		{name: "unsupported version", content: `{"version":2,"dependencies":{}}`, want: checkFailed},
		{name: "valid empty lockfile", content: `{"version":1,"dependencies":{}}`, want: checkPassed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pkg := newCheckDependenciesPackage(t)
			path := filepath.Join(pkg.LocalPath, lockfile.Filename)
			if tt.content != "" {
				require.NoError(t, os.WriteFile(path, []byte(tt.content), 0o644))
			}
			lf, results := checkLockfile(pkg, map[string]string{})
			require.Len(t, results, 1)
			assert.Equal(t, tt.want, results[0].Status)
			assert.False(t, results[0].CanFix)
			if tt.want == checkPassed {
				assert.NotNil(t, lf)
			} else {
				assert.Nil(t, lf)
			}
			if tt.content != "" {
				content, err := os.ReadFile(path)
				require.NoError(t, err)
				assert.Equal(t, tt.content, string(content))
			}
		})
	}
}

func TestCheckLockfileDependencyData(t *testing.T) {
	tests := []struct {
		name   string
		key    string
		locked lockfile.LockedDependency
		want   checkStatus
	}{
		{name: "valid revision", key: "github.com/owner/repo", locked: lockfile.LockedDependency{User: "owner", Repo: "repo", Commit: strings.Repeat("a", 40)}, want: checkPassed},
		{name: "missing revision", key: "github.com/owner/repo", locked: lockfile.LockedDependency{User: "owner", Repo: "repo"}, want: checkFailed},
		{name: "short revision", key: "github.com/owner/repo", locked: lockfile.LockedDependency{User: "owner", Repo: "repo", Commit: "abc"}, want: checkFailed},
		{name: "nonhex revision", key: "github.com/owner/repo", locked: lockfile.LockedDependency{User: "owner", Repo: "repo", Commit: strings.Repeat("z", 40)}, want: checkFailed},
		{name: "unsupported integrity", key: "github.com/owner/repo", locked: lockfile.LockedDependency{User: "owner", Repo: "repo", Commit: strings.Repeat("a", 40), Integrity: "md5:abc"}, want: checkFailed},
		{name: "nonhex integrity", key: "github.com/owner/repo", locked: lockfile.LockedDependency{User: "owner", Repo: "repo", Commit: strings.Repeat("a", 40), Integrity: "sha256:" + strings.Repeat("z", 64)}, want: checkFailed},
		{name: "mismatched key", key: "github.com/other/repo", locked: lockfile.LockedDependency{User: "owner", Repo: "repo", Commit: strings.Repeat("a", 40)}, want: checkFailed},
		{name: "local revision is optional", key: "includes://local/includes", locked: lockfile.LockedDependency{Scheme: "includes", Local: "includes", User: "local", Repo: "includes"}, want: checkPassed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pkg := newCheckDependenciesPackage(t)
			lf := lockfile.New("test")
			lf.AddDependency(tt.key, tt.locked)
			if tt.locked.Local != "" {
				pkg.Dependencies = []versioning.DependencyString{"includes://local/includes"}
			} else {
				pkg.Dependencies = []versioning.DependencyString{"owner/repo"}
			}
			writeCheckLockfile(t, pkg.LocalPath, lf)
			_, results := checkLockfile(pkg, map[string]string{})
			statuses := make([]checkStatus, 0, len(results))
			for _, result := range results {
				statuses = append(statuses, result.Status)
			}
			assert.Contains(t, statuses, tt.want)
		})
	}
}

func TestCheckLockfileConsistency(t *testing.T) {
	tests := []struct {
		name        string
		declaration versioning.DependencyString
		locked      lockfile.LockedDependency
		want        checkStatus
	}{
		{name: "matching constraint", declaration: "owner/repo:1.x", locked: lockfile.LockedDependency{Constraint: ":1.x"}, want: checkPassed},
		{name: "changed constraint", declaration: "owner/repo:2.x", locked: lockfile.LockedDependency{Constraint: ":1.x"}, want: checkFailed},
		{name: "remote schemes use ordinary lock keys", declaration: "plugin://owner/repo", want: checkPassed},
		{name: "changed include path", declaration: "owner/repo/includes", want: checkFailed},
		{name: "removed direct dependency", want: checkFailed},
		{name: "transitive entries may remain", locked: lockfile.LockedDependency{Transitive: true}, want: checkPassed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pkg := newCheckDependenciesPackage(t)
			if tt.declaration != "" {
				pkg.Dependencies = []versioning.DependencyString{tt.declaration}
			}
			lf := lockfile.New("test")
			tt.locked.User, tt.locked.Repo, tt.locked.Commit = "owner", "repo", strings.Repeat("a", 40)
			lf.AddDependency("github.com/owner/repo", tt.locked)
			writeCheckLockfile(t, pkg.LocalPath, lf)
			_, results := checkLockfile(pkg, map[string]string{})
			require.Len(t, results, 1)
			assert.Equal(t, tt.want, results[0].Status)
		})
	}
	t.Run("missing locked dependency", func(t *testing.T) {
		pkg := newCheckDependenciesPackage(t)
		pkg.Dependencies = []versioning.DependencyString{"owner/missing"}
		writeCheckLockfile(t, pkg.LocalPath, lockfile.New("test"))
		_, results := checkLockfile(pkg, map[string]string{})
		require.Len(t, results, 1)
		assert.Equal(t, checkFailed, results[0].Status)
	})
}

func writeCheckLockfile(t *testing.T, root string, lf *lockfile.Lockfile) {
	t.Helper()
	data, err := json.Marshal(lf)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, lockfile.Filename), data, 0o644))
}

func TestCheckLockfileMissingOverridesSkipsComparison(t *testing.T) {
	pkg := newCheckDependenciesPackage(t)
	pkg.Dependencies = []versioning.DependencyString{"owner/missing"}
	writeCheckLockfile(t, pkg.LocalPath, lockfile.New("test"))
	lf, results := checkLockfile(pkg, nil)
	require.NotNil(t, lf)
	require.Len(t, results, 1)
	assert.Equal(t, checkSkipped, results[0].Status)
}

func TestCheckLockfileInconsistencyPreservesIntegrityChecks(t *testing.T) {
	pkg := newCheckDependenciesPackage(t)
	pkg.Dependencies = []versioning.DependencyString{"owner/repo", "owner/missing"}
	lf := lockfile.New("test")
	lf.AddDependency("github.com/owner/repo", lockfile.LockedDependency{
		User: "owner", Repo: "repo", Commit: strings.Repeat("a", 40), Integrity: "commit:" + strings.Repeat("a", 40),
	})
	writeCheckLockfile(t, pkg.LocalPath, lf)
	loaded, results := checkLockfile(pkg, map[string]string{})
	require.NotNil(t, loaded)
	require.Len(t, results, 1)
	assert.Equal(t, checkFailed, results[0].Status)
	assert.Equal(t, lf.Dependencies, loaded.Dependencies)
}

func TestCheckLockfileBrokenSymlink(t *testing.T) {
	pkg := newCheckDependenciesPackage(t)
	if err := os.Symlink("missing.lock", filepath.Join(pkg.LocalPath, lockfile.Filename)); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	lf, results := checkLockfile(pkg, map[string]string{})
	assert.Nil(t, lf)
	require.Len(t, results, 1)
	assert.Equal(t, checkFailed, results[0].Status)
}
