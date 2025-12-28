package pkgcontext

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/build/build"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
)

func TestBuildPrepareResolvesCompilerPath(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()

	pcx := &PackageContext{
		Package: pawnpackage.Package{
			LocalPath: tempDir,
			Entry:     "gamemodes/test.pwn",
			Output:    "gamemodes/test.amx",
			Build: &build.Config{
				Compiler: build.CompilerConfig{
					Path: "tools/compiler",
				},
			},
		},
	}

	config, err := pcx.buildPrepare(context.Background(), "", false, false)
	require.NoError(t, err)

	expectedPath := fs.MustAbs(filepath.Join(tempDir, "tools/compiler"))
	require.Equal(t, expectedPath, config.Compiler.Path)
}

func TestBuildPrepareRejectsMixedCompilerConfig(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()

	pcx := &PackageContext{
		Package: pawnpackage.Package{
			LocalPath: tempDir,
			Entry:     "gamemodes/test.pwn",
			Output:    "gamemodes/test.amx",
			Build: &build.Config{
				Compiler: build.CompilerConfig{
					Path:    "tools/compiler",
					Version: "v3.10.11",
				},
			},
		},
	}

	_, err := pcx.buildPrepare(context.Background(), "", false, false)
	require.Error(t, err)
	require.Contains(t, err.Error(), "compiler.path")
}

func TestBuildPrepareAddsLocalComponentAndPluginIncludePaths(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, "components", "test", "include"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(tempDir, "components", "test", "pawn.json"),
		[]byte(`{"include_path":"include"}`),
		0o644,
	))

	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, "plugins", "plug"), 0o755))

	pcx := &PackageContext{
		Package: pawnpackage.Package{
			LocalPath: tempDir,
			Entry:     "gamemodes/test.pwn",
			Output:    "gamemodes/test.amx",
			Build:     &build.Config{},
		},
		AllPlugins: []versioning.DependencyMeta{
			{Scheme: "component", Local: "components/test", User: "local", Repo: "test"},
			{Scheme: "plugin", Local: "plugins/plug", User: "local", Repo: "plug"},
		},
	}

	config, err := pcx.buildPrepare(context.Background(), "", false, false)
	require.NoError(t, err)

	require.Contains(t, config.Includes, filepath.Join(tempDir, "components", "test", "include"))
	require.Contains(t, config.Includes, filepath.Join(tempDir, "plugins", "plug"))
}

func TestBuildPrepareKeepsLegacyDependencyIncludePathsWhenComponentSchemePresent(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()

	writePkg := func(dir string, deps []string) {
		require.NoError(t, os.MkdirAll(dir, 0o755))
		require.NoError(t, os.WriteFile(
			filepath.Join(dir, "pawn.json"),
			[]byte(`{"entry":"gamemodes/test.pwn","output":"gamemodes/test.amx","dependencies":`+toJSONArray(deps)+`}`),
			0o644,
		))
	}

	legacyDir := filepath.Join(t.TempDir(), "legacy")
	writePkg(legacyDir, []string{"pawn-lang/samp-stdlib"})

	pcxLegacy, err := NewPackageContext(gh, gitAuth, true, legacyDir, runtime.GOOS, cacheDir, "", false)
	require.NoError(t, err)

	configLegacy, err := pcxLegacy.buildPrepare(context.Background(), "", false, false)
	require.NoError(t, err)

	legacyDepIncludes := filterIncludes(configLegacy.Includes, func(p string) bool {
		return strings.Contains(p, string(filepath.Separator)+"dependencies"+string(filepath.Separator))
	})
	require.NotEmpty(t, legacyDepIncludes)

	mixedDir := filepath.Join(t.TempDir(), "mixed")
	writePkg(mixedDir, []string{"pawn-lang/samp-stdlib", "component://local/components/test"})
	require.NoError(t, os.MkdirAll(filepath.Join(mixedDir, "components", "test"), 0o755))

	pcxMixed, err := NewPackageContext(gh, gitAuth, true, mixedDir, runtime.GOOS, cacheDir, "", false)
	require.NoError(t, err)

	configMixed, err := pcxMixed.buildPrepare(context.Background(), "", false, false)
	require.NoError(t, err)

	legacyVendor := filepath.Join(legacyDir, "dependencies")
	mixedVendor := filepath.Join(mixedDir, "dependencies")
	for _, legacyInc := range legacyDepIncludes {
		require.True(t, strings.HasPrefix(legacyInc, legacyVendor+string(filepath.Separator)), "unexpected legacy include not under vendor: %s", legacyInc)
		rel, relErr := filepath.Rel(legacyVendor, legacyInc)
		require.NoError(t, relErr)
		expected := filepath.Join(mixedVendor, rel)
		require.Contains(t, configMixed.Includes, expected)
	}
}

func TestBuildPrepareKeepsResourceIncludePathsWhenComponentSchemePresent(t *testing.T) {
	if gh == nil {
		t.Skip("no GitHub client configured (FULL_ACCESS_GITHUB_TOKEN unset)")
	}

	cacheDir := t.TempDir()

	writePkg := func(dir string, deps []string) {
		require.NoError(t, os.MkdirAll(dir, 0o755))
		require.NoError(t, os.WriteFile(
			filepath.Join(dir, "pawn.json"),
			[]byte(`{"entry":"gamemodes/test.pwn","output":"gamemodes/test.amx","dependencies":`+toJSONArray(deps)+`}`),
			0o644,
		))
	}

	baseDir := filepath.Join(t.TempDir(), "base")
	writePkg(baseDir, []string{"sampctl/package-resource-test"})

	pcxBase, err := NewPackageContext(gh, gitAuth, true, baseDir, runtime.GOOS, cacheDir, "", false)
	require.NoError(t, err)

	configBase, err := pcxBase.buildPrepare(context.Background(), "", true, false)
	require.NoError(t, err)

	baseVendor := filepath.Join(baseDir, "dependencies")
	baseResourceIncludes := filterIncludes(configBase.Includes, func(p string) bool {
		return strings.Contains(p, string(filepath.Separator)+".resources"+string(filepath.Separator)) &&
			strings.Contains(p, "package-resource-test")
	})
	require.NotEmpty(t, baseResourceIncludes)
	for _, inc := range baseResourceIncludes {
		require.True(t, fs.Exists(inc), "expected resource include dir to exist: %s", inc)
	}

	mixedDir := filepath.Join(t.TempDir(), "mixed")
	writePkg(mixedDir, []string{"sampctl/package-resource-test", "component://local/components/test"})
	require.NoError(t, os.MkdirAll(filepath.Join(mixedDir, "components", "test"), 0o755))

	pcxMixed, err := NewPackageContext(gh, gitAuth, true, mixedDir, runtime.GOOS, cacheDir, "", false)
	require.NoError(t, err)

	configMixed, err := pcxMixed.buildPrepare(context.Background(), "", true, false)
	require.NoError(t, err)

	mixedVendor := filepath.Join(mixedDir, "dependencies")
	for _, baseInc := range baseResourceIncludes {
		rel, relErr := filepath.Rel(baseVendor, baseInc)
		require.NoError(t, relErr)

		expected := filepath.Join(mixedVendor, rel)
		require.Contains(t, configMixed.Includes, expected)
		require.True(t, fs.Exists(expected), "expected resource include dir to exist: %s", expected)
	}
}

func toJSONArray(values []string) string {
	if len(values) == 0 {
		return "[]"
	}
	var b strings.Builder
	b.WriteString("[")
	for i, v := range values {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString("\"")
		b.WriteString(strings.ReplaceAll(v, "\"", "\\\""))
		b.WriteString("\"")
	}
	b.WriteString("]")
	return b.String()
}

func filterIncludes(includes []string, keep func(string) bool) []string {
	var out []string
	for _, inc := range includes {
		if keep(inc) {
			out = append(out, inc)
		}
	}
	return out
}
