package pkgcontext

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
)

func TestHandleURLSchemeCaching(t *testing.T) {
	for _, tc := range []struct {
		name         string
		meta         versioning.DependencyMeta
		wantPlugins  int
		wantIncludes int
	}{
		{
			name:        "local plugin",
			meta:        versioning.DependencyMeta{Scheme: "plugin", Local: "plugins/test"},
			wantPlugins: 1,
		},
		{
			name:        "remote plugin",
			meta:        versioning.DependencyMeta{Scheme: "plugin", Site: "github.com", User: "user", Repo: "plugin-repo", Tag: "v1.0.0"},
			wantPlugins: 1,
		},
		{
			name:        "local component",
			meta:        versioning.DependencyMeta{Scheme: "component", Local: "components/test"},
			wantPlugins: 1,
		},
		{
			name:        "remote component",
			meta:        versioning.DependencyMeta{Scheme: "component", Site: "github.com", User: "user", Repo: "component-repo", Tag: "v1.0.0"},
			wantPlugins: 1,
		},
		{
			name:         "local includes",
			meta:         versioning.DependencyMeta{Scheme: "includes", Local: "legacy/includes"},
			wantIncludes: 1,
		},
		{
			name:         "remote includes",
			meta:         versioning.DependencyMeta{Scheme: "includes", Site: "github.com", User: "user", Repo: "includes-repo"},
			wantIncludes: 1,
		},
		{
			name: "local filterscript",
			meta: versioning.DependencyMeta{Scheme: "filterscript", Local: "filterscripts/test.pwn"},
		},
		{
			name: "remote filterscript",
			meta: versioning.DependencyMeta{Scheme: "filterscript", Site: "github.com", User: "user", Repo: "filterscript-repo"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pcx := &PackageContext{Package: pawnpackage.Package{
				LocalPath: "/test/path",
				Vendor:    "/test/path/dependencies",
			}}

			err := pcx.handleURLSchemeCaching(tc.meta, "test-prefix")

			assert.NoError(t, err)
			assert.Len(t, pcx.AllPlugins, tc.wantPlugins, "unexpected number of plugins")
			assert.Len(t, pcx.AllIncludePaths, tc.wantIncludes, "unexpected number of include paths")
			assert.Empty(t, pcx.AllDependencies)
		})
	}
}

func TestHandleURLSchemeCachingRejectsUnsupportedScheme(t *testing.T) {
	pcx := &PackageContext{Package: pawnpackage.Package{LocalPath: "/test/path"}}
	meta := versioning.DependencyMeta{Scheme: "unsupported", User: "user", Repo: "repo"}

	err := pcx.handleURLSchemeCaching(meta, "test-prefix")

	assert.Error(t, err)
}

func TestEnsureLocalURLSchemeDependency(t *testing.T) {
	for _, tc := range []struct {
		scheme   string
		path     string
		contents string
	}{
		{scheme: "plugin", path: "plugins/test", contents: "mock plugin"},
		{scheme: "component", path: "components/test", contents: "mock component"},
		{scheme: "includes", path: "legacy/includes", contents: "mock include"},
		{scheme: "filterscript", path: "filterscripts/test.pwn", contents: "mock filterscript"},
	} {
		t.Run(tc.scheme, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, tc.path)
			require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
			require.NoError(t, os.WriteFile(path, []byte(tc.contents), 0o644))
			pcx := &PackageContext{Package: pawnpackage.Package{LocalPath: root}}
			meta := versioning.DependencyMeta{Scheme: tc.scheme, Local: tc.path}

			err := pcx.ensureURLSchemeDependency(t.Context(), meta)

			assert.NoError(t, err)
		})
	}
}

func TestEnsureURLSchemeDependencyRejectsUnsupportedScheme(t *testing.T) {
	pcx := &PackageContext{Package: pawnpackage.Package{LocalPath: t.TempDir()}}
	meta := versioning.DependencyMeta{Scheme: "unsupported", User: "user", Repo: "repo"}

	err := pcx.ensureURLSchemeDependency(t.Context(), meta)

	assert.Error(t, err)
}
