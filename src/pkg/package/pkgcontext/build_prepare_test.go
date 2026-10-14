package pkgcontext

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/build"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
)

func TestBuildPreparePreservesConfigIdentityAndIncludeOrder(t *testing.T) {
	root := t.TempDir()
	dependencyDir := filepath.Join(root, "dependencies", "dependency")
	pluginDir := filepath.Join(root, "plugins", "local")
	componentDir := filepath.Join(root, "components", "local")
	writeBuildPreparePackage(t, dependencyDir, `{"include_path":"include"}`)
	writeBuildPreparePackage(t, pluginDir, `{"include_path":"pawn"}`)
	writeBuildPreparePackage(t, componentDir, `{`)

	config := &build.Config{Includes: []string{root, "configured", "configured"}}
	resourceInclude := filepath.Join(root, "resource-include")
	dependency := versioning.DependencyMeta{User: "fixture", Repo: "dependency"}
	pcx := &PackageContext{
		Package: pawnpackage.Package{LocalPath: root, Build: config},
		PackageResolvedState: PackageResolvedState{
			AllDependencies: []versioning.DependencyMeta{dependency, dependency},
			AllPlugins: []versioning.DependencyMeta{
				{Scheme: "plugin", Local: "plugins/local"},
				{Scheme: "plugin", User: "fixture", Repo: "remote"},
				{Scheme: "includes", Local: "ignored-non-plugin"},
				{Scheme: "component", Local: "components/local"},
			},
			AllIncludePaths: []string{resourceInclude, resourceInclude},
		},
	}

	prepared, err := pcx.buildPrepare(context.Background(), "", false, false)
	require.NoError(t, err)
	require.Same(t, config, prepared)
	assert.Equal(t, []string{
		root, "configured", "configured", root,
		filepath.Join(dependencyDir, "include"), filepath.Join(dependencyDir, "include"),
		filepath.Join(pluginDir, "pawn"), componentDir,
		resourceInclude, resourceInclude,
	}, prepared.Includes)
}

func TestBuildPrepareDependencyDefinitionFallback(t *testing.T) {
	tests := []struct {
		name            string
		localDefinition string
		cacheDefinition string
		includePath     string
		suppressed      bool
	}{
		{
			name:            "local definition wins",
			localDefinition: `{"include_path":"local"}`,
			cacheDefinition: `{"include_path":"cached"}`,
			includePath:     "local",
		},
		{
			name:            "missing local definition does not consult cache",
			cacheDefinition: `{"include_path":"cached"}`,
		},
		{
			name:            "malformed local definition uses cache",
			localDefinition: `{`,
			cacheDefinition: `{"include_path":"cached"}`,
			includePath:     "cached",
		},
		{
			name:            "malformed local and cached definitions use dependency root",
			localDefinition: `{`,
			cacheDefinition: `{`,
		},
		{
			name:            "local include resources suppress ordinary includes on any platform",
			localDefinition: `{"include_path":"local","resources":[{"name":"archive.zip","platform":"other-platform","includes":["include"]}]}`,
			suppressed:      true,
		},
		{
			name:            "cached include resources suppress ordinary includes",
			localDefinition: `{`,
			cacheDefinition: `{"resources":[{"name":"archive.zip","platform":"other-platform","includes":["include"]}]}`,
			suppressed:      true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root, cache := t.TempDir(), t.TempDir()
			dependency := versioning.DependencyMeta{User: "fixture", Repo: "dependency"}
			dependencyDir := filepath.Join(root, "dependencies", dependency.Repo)
			if test.localDefinition != "" {
				writeBuildPreparePackage(t, dependencyDir, test.localDefinition)
			}
			if test.cacheDefinition != "" {
				writeBuildPreparePackage(t, dependency.CachePath(cache), test.cacheDefinition)
			}

			pcx := &PackageContext{
				Package:              pawnpackage.Package{LocalPath: root, Build: &build.Config{}},
				PackageServices:      PackageServices{CacheDir: cache, Platform: "linux"},
				PackageResolvedState: PackageResolvedState{AllDependencies: []versioning.DependencyMeta{dependency}},
			}
			prepared, err := pcx.buildPrepare(context.Background(), "", false, false)
			require.NoError(t, err)

			expected := []string{root}
			if !test.suppressed {
				expected = append(expected, filepath.Join(dependencyDir, test.includePath))
			}
			assert.Equal(t, expected, prepared.Includes)
		})
	}
}

func TestBuildPrepareMissingPluginPreservesPartialConfig(t *testing.T) {
	root := t.TempDir()
	pluginDir := filepath.Join(root, "plugins", "present")
	writeBuildPreparePackage(t, pluginDir, `{"include_path":"include"}`)
	config := &build.Config{Includes: []string{"configured"}}
	pcx := &PackageContext{
		Package: pawnpackage.Package{LocalPath: root, Build: config},
		PackageResolvedState: PackageResolvedState{
			AllDependencies: []versioning.DependencyMeta{{Repo: "dependency"}},
			AllPlugins: []versioning.DependencyMeta{
				{Scheme: "plugin", Local: "plugins/present"},
				{Scheme: "component", Local: "components/missing"},
				{Scheme: "plugin", Local: "plugins/also-missing"},
			},
			AllIncludePaths: []string{"trailing-resource"},
		},
	}

	prepared, err := pcx.buildPrepare(context.Background(), "", false, false)
	require.EqualError(t, err, "local component dependency path does not exist: "+filepath.Join(root, "components", "missing"))
	require.Same(t, config, prepared)
	assert.Equal(t, []string{
		"configured", root, filepath.Join(root, "dependencies", "dependency"), filepath.Join(pluginDir, "include"),
	}, prepared.Includes)
}

func TestBuildPrepareValidatesBeforePathMutationAndFileGeneration(t *testing.T) {
	root := t.TempDir()
	config := &build.Config{
		Input:      "relative-input.pwn",
		Output:     "relative-output.amx",
		WorkingDir: "relative-directory",
		Includes:   []string{"configured"},
		Compiler:   build.CompilerConfig{Path: "relative-compiler", Version: "conflicting-version"},
	}
	original := *config
	pcx := &PackageContext{Package: pawnpackage.Package{LocalPath: root, Build: config}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	prepared, err := pcx.buildPrepare(ctx, "", true, true)
	require.EqualError(t, err, "invalid compiler configuration: compiler.path cannot be combined with compiler.site, compiler.user, compiler.repo, compiler.version, or compiler.preset")
	require.Same(t, config, prepared)
	assert.Equal(t, original, *config)
	_, err = os.Stat(filepath.Join(root, "sampctl_build_file.inc"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestBuildPrepareGeneratesFileBeforeDependencyCancellation(t *testing.T) {
	for _, ensure := range []bool{false, true} {
		name := "without ensure"
		if ensure {
			name = "with ensure"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			config := &build.Config{
				Input:     "main.pwn",
				Includes:  []string{"configured"},
				Constants: map[string]string{"BEFORE_ENSURE": "42"},
			}
			pcx := &PackageContext{
				Package: pawnpackage.Package{LocalPath: root, Build: config},
				PackageResolvedState: PackageResolvedState{
					AllDependencies: []versioning.DependencyMeta{{Repo: "dependency"}},
					AllIncludePaths: []string{"trailing-resource"},
				},
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			prepared, err := pcx.buildPrepare(ctx, "", ensure, false)
			require.Same(t, config, prepared)
			assert.Equal(t, filepath.Join(root, "main.pwn"), config.Input)
			expectedIncludes := []string{"configured", root}
			if ensure {
				require.ErrorIs(t, err, context.Canceled)
				require.ErrorContains(t, err, "failed to ensure dependencies before build")
			} else {
				require.NoError(t, err)
				expectedIncludes = append(expectedIncludes, filepath.Join(root, "dependencies", "dependency"), "trailing-resource")
			}
			assert.Equal(t, expectedIncludes, prepared.Includes)

			contents, err := os.ReadFile(filepath.Join(root, "sampctl_build_file.inc"))
			require.NoError(t, err)
			assert.Contains(t, string(contents), "#define BEFORE_ENSURE 42")
		})
	}
}

func writeBuildPreparePackage(t *testing.T, dir, definition string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pawn.json"), []byte(definition), 0o600))
}
