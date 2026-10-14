package versioning

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCachePathIncludesNormalizedHost(t *testing.T) {
	t.Parallel()

	meta := DependencyMeta{User: "fixture", Repo: "shared"}
	root := t.TempDir()
	defaultPath := meta.CachePath(root)
	require.Equal(t, filepath.Join(root, "packages", "github.com", "fixture", "shared", "default"), defaultPath)

	meta.Site = "GitHub.COM"
	require.Equal(t, defaultPath, meta.CachePath(root))
	meta.Site = "gitlab.com"
	require.NotEqual(t, defaultPath, meta.CachePath(root))

	meta.Site = "git.example.com:8443"
	require.Equal(t, filepath.Join(root, "packages", "git.example.com%3A8443", "fixture", "shared", "default"), meta.CachePath(root))
	meta.Site = ".."
	require.Equal(t, filepath.Join(root, "packages", "%2E%2E", "fixture", "shared", "default"), meta.CachePath(root))
}
