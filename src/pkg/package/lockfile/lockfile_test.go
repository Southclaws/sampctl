package lockfile

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
)

func TestNew(t *testing.T) {
	lf := New("1.0.0")

	assert.Equal(t, Version, lf.Version)
	assert.Equal(t, "1.0.0", lf.SampctlVersion)
	assert.NotNil(t, lf.Dependencies)
	assert.Empty(t, lf.Dependencies)
	assert.False(t, lf.Generated.IsZero())
}

func TestDependencyKey(t *testing.T) {
	tests := []struct {
		name     string
		meta     versioning.DependencyMeta
		expected string
	}{
		{
			name: "simple github dependency",
			meta: versioning.DependencyMeta{
				Site: "github.com",
				User: "user",
				Repo: "repo",
			},
			expected: "github.com/user/repo",
		},
		{
			name: "default site",
			meta: versioning.DependencyMeta{
				User: "user",
				Repo: "repo",
			},
			expected: "github.com/user/repo",
		},
		{
			name: "plugin scheme local",
			meta: versioning.DependencyMeta{
				Scheme: "plugin",
				Local:  "plugins/test",
			},
			expected: "plugin://local/plugins/test",
		},
		{
			name: "plugin scheme remote",
			meta: versioning.DependencyMeta{
				Scheme: "plugin",
				User:   "user",
				Repo:   "plugin-repo",
			},
			expected: "plugin://user/plugin-repo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DependencyKey(tt.meta)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestLockfile_AddAndGetDependency(t *testing.T) {
	lf := New("1.0.0")

	dep := LockedDependency{
		Constraint: ":1.2.3",
		Resolved:   "1.2.3",
		Commit:     "abc123def456789012345678901234567890abcd",
		User:       "user",
		Repo:       "repo",
	}

	lf.AddDependency("github.com/user/repo", dep)

	assert.Equal(t, 1, lf.DependencyCount())

	retrieved, ok := lf.GetDependency("github.com/user/repo")
	assert.True(t, ok)
	assert.Equal(t, dep.Commit, retrieved.Commit)
	assert.Equal(t, dep.Resolved, retrieved.Resolved)

	_, ok = lf.GetDependency("github.com/nonexistent/repo")
	assert.False(t, ok)
}

func TestLockfile_HasDependency(t *testing.T) {
	lf := New("1.0.0")

	dep := LockedDependency{
		User: "user",
		Repo: "repo",
	}
	lf.AddDependency("github.com/user/repo", dep)

	assert.True(t, lf.HasDependency("github.com/user/repo"))
	assert.False(t, lf.HasDependency("github.com/other/repo"))
}

func TestLockfile_RemoveDependency(t *testing.T) {
	lf := New("1.0.0")

	lf.AddDependency("github.com/user/repo", LockedDependency{})
	assert.Equal(t, 1, lf.DependencyCount())

	lf.RemoveDependency("github.com/user/repo")
	assert.Equal(t, 0, lf.DependencyCount())
}

func TestLockfile_DirectAndTransitiveDependencies(t *testing.T) {
	lf := New("1.0.0")

	lf.AddDependency("github.com/user/direct", LockedDependency{
		User:       "user",
		Repo:       "direct",
		Transitive: false,
	})
	lf.AddDependency("github.com/user/transitive", LockedDependency{
		User:       "user",
		Repo:       "transitive",
		Transitive: true,
		RequiredBy: []string{"github.com/user/direct"},
	})

	direct := lf.DirectDependencies()
	transitive := lf.TransitiveDependencies()

	assert.Len(t, direct, 1)
	assert.Len(t, transitive, 1)
	assert.Contains(t, direct, "github.com/user/direct")
	assert.Contains(t, transitive, "github.com/user/transitive")
}

func TestLockfile_GetLockedMeta(t *testing.T) {
	lf := New("1.0.0")

	commitSHA := "abc123def456789012345678901234567890abcd"
	lf.AddDependency("github.com/user/repo", LockedDependency{
		Constraint: ":1.x",
		Resolved:   "1.2.3",
		Commit:     commitSHA,
		User:       "user",
		Repo:       "repo",
	})

	meta := versioning.DependencyMeta{
		User: "user",
		Repo: "repo",
		Tag:  "1.x",
	}

	lockedMeta, ok := lf.GetLockedMeta(meta)
	assert.True(t, ok)
	assert.Equal(t, commitSHA, lockedMeta.Commit)
	assert.Empty(t, lockedMeta.Tag)
	assert.Empty(t, lockedMeta.Branch)
}

func TestLockfile_Validate(t *testing.T) {
	tests := []struct {
		name      string
		lockfile  *Lockfile
		expectErr bool
	}{
		{
			name:      "valid lockfile",
			lockfile:  New("1.0.0"),
			expectErr: false,
		},
		{
			name:      "missing version",
			lockfile:  &Lockfile{},
			expectErr: true,
		},
		{
			name: "future version",
			lockfile: &Lockfile{
				Version: Version + 1,
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.lockfile.Validate()
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()

	lf := New("1.0.0")
	lf.AddDependency("github.com/user/repo", LockedDependency{
		Constraint: ":1.2.3",
		Resolved:   "1.2.3",
		Commit:     "abc123def456789012345678901234567890abcd",
		User:       "user",
		Repo:       "repo",
	})

	// Test JSON format (default)
	err := Save(tmpDir, lf, "json")
	require.NoError(t, err)

	assert.FileExists(t, filepath.Join(tmpDir, Filename))

	loaded, err := Load(tmpDir)
	require.NoError(t, err)
	require.NotNil(t, loaded)

	assert.Equal(t, lf.Version, loaded.Version)
	assert.Equal(t, lf.SampctlVersion, loaded.SampctlVersion)
	assert.Equal(t, lf.DependencyCount(), loaded.DependencyCount())

	// Clean up and test YAML format (same filename, different encoding)
	os.Remove(filepath.Join(tmpDir, Filename))

	err = Save(tmpDir, lf, "yaml")
	require.NoError(t, err)

	assert.FileExists(t, filepath.Join(tmpDir, Filename))

	loaded, err = Load(tmpDir)
	require.NoError(t, err)
	require.NotNil(t, loaded)

	assert.Equal(t, lf.DependencyCount(), loaded.DependencyCount())
}

func TestExists(t *testing.T) {
	tmpDir := t.TempDir()

	assert.False(t, Exists(tmpDir))

	lf := New("1.0.0")
	err := Save(tmpDir, lf, "json")
	require.NoError(t, err)

	assert.True(t, Exists(tmpDir))
}

func TestDelete(t *testing.T) {
	tmpDir := t.TempDir()

	lf := New("1.0.0")
	err := Save(tmpDir, lf, "json")
	require.NoError(t, err)

	assert.True(t, Exists(tmpDir))

	err = Delete(tmpDir)
	require.NoError(t, err)

	assert.False(t, Exists(tmpDir))
}

func TestLoadOrCreate(t *testing.T) {
	tmpDir := t.TempDir()

	// Should create new when not exists
	lf, err := LoadOrCreate(tmpDir, "1.0.0")
	require.NoError(t, err)
	require.NotNil(t, lf)
	assert.Equal(t, "1.0.0", lf.SampctlVersion)
	assert.Empty(t, lf.Dependencies)

	// Save it
	lf.AddDependency("github.com/user/repo", LockedDependency{User: "user", Repo: "repo"})
	err = Save(tmpDir, lf, "json")
	require.NoError(t, err)

	// Should load existing
	loaded, err := LoadOrCreate(tmpDir, "2.0.0")
	require.NoError(t, err)
	assert.Equal(t, "1.0.0", loaded.SampctlVersion) // Should retain original version
	assert.Equal(t, 1, loaded.DependencyCount())
}

func TestUpdateTimestamp(t *testing.T) {
	lf := New("1.0.0")
	originalTime := lf.Generated

	time.Sleep(10 * time.Millisecond)
	lf.UpdateTimestamp()

	assert.True(t, lf.Generated.After(originalTime))
}