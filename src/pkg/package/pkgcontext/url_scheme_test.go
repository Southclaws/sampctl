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
	tests := []struct {
		name             string
		pcx              *PackageContext
		meta             versioning.DependencyMeta
		expectedPlugins  int
		expectedIncludes int
		expectedDeps     int
		wantErr          bool
	}{
		{
			name: "local plugin scheme",
			pcx: &PackageContext{
				Package: pawnpackage.Package{
					LocalPath: "/test/path",
				},
				AllPlugins:      []versioning.DependencyMeta{},
				AllIncludePaths: []string{},
				AllDependencies: []versioning.DependencyMeta{},
			},
			meta: versioning.DependencyMeta{
				Scheme: "plugin",
				Local:  "plugins/test",
			},
			expectedPlugins:  1,
			expectedIncludes: 0,
			expectedDeps:     0,
			wantErr:          false,
		},
		{
			name: "remote plugin scheme",
			pcx: &PackageContext{
				Package: pawnpackage.Package{
					LocalPath: "/test/path",
				},
				AllPlugins:      []versioning.DependencyMeta{},
				AllIncludePaths: []string{},
				AllDependencies: []versioning.DependencyMeta{},
			},
			meta: versioning.DependencyMeta{
				Scheme: "plugin",
				Site:   "github.com",
				User:   "user",
				Repo:   "plugin-repo",
				Tag:    "v1.0.0",
			},
			expectedPlugins:  1,
			expectedIncludes: 0,
			expectedDeps:     1,
			wantErr:          false,
		},
		{
			name: "local component scheme",
			pcx: &PackageContext{
				Package: pawnpackage.Package{
					LocalPath: "/test/path",
				},
				AllPlugins:      []versioning.DependencyMeta{},
				AllIncludePaths: []string{},
				AllDependencies: []versioning.DependencyMeta{},
			},
			meta: versioning.DependencyMeta{
				Scheme: "component",
				Local:  "components/test",
			},
			expectedPlugins:  1,
			expectedIncludes: 0,
			expectedDeps:     0,
			wantErr:          false,
		},
		{
			name: "remote component scheme",
			pcx: &PackageContext{
				Package: pawnpackage.Package{
					LocalPath: "/test/path",
				},
				AllPlugins:      []versioning.DependencyMeta{},
				AllIncludePaths: []string{},
				AllDependencies: []versioning.DependencyMeta{},
			},
			meta: versioning.DependencyMeta{
				Scheme: "component",
				Site:   "github.com",
				User:   "user",
				Repo:   "component-repo",
				Tag:    "v1.0.0",
			},
			expectedPlugins:  1,
			expectedIncludes: 0,
			expectedDeps:     1,
			wantErr:          false,
		},
		{
			name: "local includes scheme",
			pcx: &PackageContext{
				Package: pawnpackage.Package{
					LocalPath: "/test/path",
				},
				AllPlugins:      []versioning.DependencyMeta{},
				AllIncludePaths: []string{},
				AllDependencies: []versioning.DependencyMeta{},
			},
			meta: versioning.DependencyMeta{
				Scheme: "includes",
				Local:  "legacy/includes",
			},
			expectedPlugins:  0,
			expectedIncludes: 1,
			expectedDeps:     0,
			wantErr:          false,
		},
		{
			name: "remote includes scheme",
			pcx: &PackageContext{
				Package: pawnpackage.Package{
					LocalPath: "/test/path",
					Vendor:    "/test/path/dependencies",
				},
				AllPlugins:      []versioning.DependencyMeta{},
				AllIncludePaths: []string{},
				AllDependencies: []versioning.DependencyMeta{},
			},
			meta: versioning.DependencyMeta{
				Scheme: "includes",
				Site:   "github.com",
				User:   "user",
				Repo:   "includes-repo",
			},
			expectedPlugins:  0,
			expectedIncludes: 1,
			expectedDeps:     1,
			wantErr:          false,
		},
		{
			name: "local filterscript scheme",
			pcx: &PackageContext{
				Package: pawnpackage.Package{
					LocalPath: "/test/path",
				},
				AllPlugins:      []versioning.DependencyMeta{},
				AllIncludePaths: []string{},
				AllDependencies: []versioning.DependencyMeta{},
			},
			meta: versioning.DependencyMeta{
				Scheme: "filterscript",
				Local:  "filterscripts/test.pwn",
			},
			expectedPlugins:  0,
			expectedIncludes: 0,
			expectedDeps:     0,
			wantErr:          false,
		},
		{
			name: "remote filterscript scheme",
			pcx: &PackageContext{
				Package: pawnpackage.Package{
					LocalPath: "/test/path",
				},
				AllPlugins:      []versioning.DependencyMeta{},
				AllIncludePaths: []string{},
				AllDependencies: []versioning.DependencyMeta{},
			},
			meta: versioning.DependencyMeta{
				Scheme: "filterscript",
				Site:   "github.com",
				User:   "user",
				Repo:   "filterscript-repo",
			},
			expectedPlugins:  0,
			expectedIncludes: 0,
			expectedDeps:     1,
			wantErr:          false,
		},
		{
			name: "unsupported scheme",
			pcx: &PackageContext{
				Package: pawnpackage.Package{
					LocalPath: "/test/path",
				},
				AllPlugins:      []versioning.DependencyMeta{},
				AllIncludePaths: []string{},
				AllDependencies: []versioning.DependencyMeta{},
			},
			meta: versioning.DependencyMeta{
				Scheme: "unsupported",
				User:   "user",
				Repo:   "repo",
			},
			expectedPlugins:  0,
			expectedIncludes: 0,
			expectedDeps:     0,
			wantErr:          true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.pcx.handleURLSchemeCaching(tt.meta, "test-prefix")

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Len(t, tt.pcx.AllPlugins, tt.expectedPlugins, "unexpected number of plugins")
			assert.Len(t, tt.pcx.AllIncludePaths, tt.expectedIncludes, "unexpected number of include paths")
			assert.Len(t, tt.pcx.AllDependencies, tt.expectedDeps, "unexpected number of dependencies")
		})
	}
}

func TestEnsureURLSchemeDependency(t *testing.T) {
	// Create temporary directory for test files
	tmpDir, err := os.MkdirTemp("", "sampctl-test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	testPath := filepath.Join(tmpDir, "test", "path")

	// Create necessary directories and files for the tests
	require.NoError(t, os.MkdirAll(filepath.Join(testPath, "plugins"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(testPath, "components"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(testPath, "legacy"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(testPath, "filterscripts"), 0o755))

	// Create mock files
	require.NoError(t, os.WriteFile(filepath.Join(testPath, "plugins", "test"), []byte("mock plugin"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(testPath, "components", "test"), []byte("mock component"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(testPath, "legacy", "includes"), []byte("mock include"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(testPath, "filterscripts", "test.pwn"), []byte("mock filterscript"), 0o644))

	tests := []struct {
		name    string
		pcx     *PackageContext
		meta    versioning.DependencyMeta
		wantErr bool
	}{
		{
			name: "plugin scheme",
			pcx: &PackageContext{
				Package: pawnpackage.Package{
					LocalPath: testPath,
				},
				AllPlugins:      []versioning.DependencyMeta{},
				AllIncludePaths: []string{},
			},
			meta: versioning.DependencyMeta{
				Scheme: "plugin",
				Local:  "plugins/test",
			},
			wantErr: false,
		},
		{
			name: "includes scheme",
			pcx: &PackageContext{
				Package: pawnpackage.Package{
					LocalPath: testPath,
				},
				AllPlugins:      []versioning.DependencyMeta{},
				AllIncludePaths: []string{},
			},
			meta: versioning.DependencyMeta{
				Scheme: "includes",
				Local:  "legacy/includes",
			},
			wantErr: false,
		},
		{
			name: "component scheme",
			pcx: &PackageContext{
				Package: pawnpackage.Package{
					LocalPath: testPath,
				},
				AllPlugins:      []versioning.DependencyMeta{},
				AllIncludePaths: []string{},
			},
			meta: versioning.DependencyMeta{
				Scheme: "component",
				Local:  "components/test",
			},
			wantErr: false,
		},
		{
			name: "filterscript scheme",
			pcx: &PackageContext{
				Package: pawnpackage.Package{
					LocalPath: testPath,
				},
				AllPlugins:      []versioning.DependencyMeta{},
				AllIncludePaths: []string{},
			},
			meta: versioning.DependencyMeta{
				Scheme: "filterscript",
				Local:  "filterscripts/test.pwn",
			},
			wantErr: false,
		},
		{
			name: "unsupported scheme",
			pcx: &PackageContext{
				Package: pawnpackage.Package{
					LocalPath: testPath,
				},
				AllPlugins:      []versioning.DependencyMeta{},
				AllIncludePaths: []string{},
			},
			meta: versioning.DependencyMeta{
				Scheme: "unsupported",
				User:   "user",
				Repo:   "repo",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.pcx.ensureURLSchemeDependency(tt.meta)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
