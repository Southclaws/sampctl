package pkgcontext

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/fsnotify/fsnotify"
	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/build"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
)

func boolPtr(v bool) *bool {
	return &v
}

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

func TestBuildPrepareResolvesPathsFromPackageRoot(t *testing.T) {
	tempDir := t.TempDir()
	otherDir := t.TempDir()

	previousWD, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(otherDir))
	t.Cleanup(func() {
		require.NoError(t, os.Chdir(previousWD))
	})

	pcx := &PackageContext{
		Package: pawnpackage.Package{
			LocalPath: tempDir,
			Entry:     "gamemodes/test.pwn",
			Output:    "gamemodes/test.amx",
			Build: &build.Config{
				Input:      "filterscripts/custom.pwn",
				Output:     "artifacts/custom.amx",
				WorkingDir: "filterscripts",
			},
		},
	}

	config, err := pcx.buildPrepare(context.Background(), "", false, false)
	require.NoError(t, err)

	require.Equal(t, filepath.Join(tempDir, "filterscripts", "custom.pwn"), config.Input)
	require.Equal(t, filepath.Join(tempDir, "artifacts", "custom.amx"), config.Output)
	require.Equal(t, filepath.Join(tempDir, "filterscripts"), config.WorkingDir)
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
		PackageResolvedState: PackageResolvedState{AllPlugins: []versioning.DependencyMeta{
			{Scheme: "component", Local: "components/test", User: "local", Repo: "test"},
			{Scheme: "plugin", Local: "plugins/plug", User: "local", Repo: "plug"},
		}},
	}

	config, err := pcx.buildPrepare(context.Background(), "", false, false)
	require.NoError(t, err)

	require.Contains(t, config.Includes, filepath.Join(tempDir, "components", "test", "include"))
	require.Contains(t, config.Includes, filepath.Join(tempDir, "plugins", "plug"))
}

func TestBuildPrepareKeepsLegacyDependencyIncludePathsWhenComponentSchemePresent(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	seedPkgContextBuildCache(t, cacheDir)

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

	pcxLegacy, err := NewPackageContext(NewPackageContextOptions{
		GitHub:   gh,
		Auth:     gitAuth,
		Parent:   true,
		Dir:      legacyDir,
		Platform: runtime.GOOS,
		CacheDir: cacheDir,
	})
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

	pcxMixed, err := NewPackageContext(NewPackageContextOptions{
		GitHub:   gh,
		Auth:     gitAuth,
		Parent:   true,
		Dir:      mixedDir,
		Platform: runtime.GOOS,
		CacheDir: cacheDir,
	})
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
	cacheDir := t.TempDir()
	seedPkgContextBuildCache(t, cacheDir)

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

	pcxBase, err := NewPackageContext(NewPackageContextOptions{
		GitHub:   gh,
		Auth:     gitAuth,
		Parent:   true,
		Dir:      baseDir,
		Platform: runtime.GOOS,
		CacheDir: cacheDir,
	})
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

	pcxMixed, err := NewPackageContext(NewPackageContextOptions{
		GitHub:   gh,
		Auth:     gitAuth,
		Parent:   true,
		Dir:      mixedDir,
		Platform: runtime.GOOS,
		CacheDir: cacheDir,
	})
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

func seedPkgContextBuildCache(t *testing.T, cacheDir string) {
	t.Helper()
	require.NoError(t, copyPkgContextFixtureDir(filepath.Join("tests", "cache", "packages"), filepath.Join(cacheDir, "packages")))
	require.NoError(t, stripPkgContextFixtureCacheRemotes(cacheDir))
}

func copyPkgContextFixtureDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)

		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}

		if info.Mode()&os.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode())
	})
}

func TestBuildPrepareGeneratesBuildFileWithConstants(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("BUILD_ENV_VALUE", "from-env")

	pcx := &PackageContext{
		PackageServices:       PackageServices{Platform: "linux"},
		PackageExecutionState: PackageExecutionState{AppVersion: "2.3.4"},
		Package: pawnpackage.Package{
			LocalPath: tempDir,
			Entry:     "gamemodes/test.pwn",
			Output:    "gamemodes/test.amx",
			Experimental: &pawnpackage.ExperimentalConfig{
				BuildFile: boolPtr(true),
			},
			Build: &build.Config{
				Constants: map[string]string{
					"NUM_CONST":    "42",
					"STR_CONST":    "hello",
					"ENV_CONST":    "$BUILD_ENV_VALUE",
					"QUOTED_CONST": "\"already quoted\"",
					"ESC_CONST":    "hello\"world",
				},
			},
		},
	}

	_, err := pcx.buildPrepare(context.Background(), "", false, false)
	require.NoError(t, err)

	buildFilePath := filepath.Join(tempDir, "sampctl_build_file.inc")
	contents, err := os.ReadFile(buildFilePath)
	require.NoError(t, err)

	text := string(contents)
	require.Contains(t, text, "#define SAMPCTL_BUILD_FILE 1")
	require.Contains(t, text, "#define SAMPCTL_VERSION \"2.3.4\"")
	require.Contains(t, text, "#define SAMPCTL_PLATFORM \"linux\"")
	require.Contains(t, text, "#define NUM_CONST 42")
	require.Contains(t, text, "#define STR_CONST \"hello\"")
	require.Contains(t, text, "#define ENV_CONST \"from-env\"")
	require.Contains(t, text, "#define QUOTED_CONST \"already quoted\"")
	require.Contains(t, text, "#define ESC_CONST \"hello\\\"world\"")
}

func TestBuildPrepareGeneratesBuildFileByDefault(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()

	pcx := &PackageContext{
		PackageServices:       PackageServices{Platform: "windows"},
		PackageExecutionState: PackageExecutionState{AppVersion: "1.2.3"},
		Package: pawnpackage.Package{
			LocalPath: tempDir,
			Entry:     "gamemodes/test.pwn",
			Output:    "gamemodes/test.amx",
			Build:     &build.Config{},
		},
	}

	_, err := pcx.buildPrepare(context.Background(), "", false, false)
	require.NoError(t, err)

	buildFilePath := filepath.Join(tempDir, "sampctl_build_file.inc")
	contents, err := os.ReadFile(buildFilePath)
	require.NoError(t, err)

	text := string(contents)
	require.Contains(t, text, "#define SAMPCTL_BUILD_FILE 1")
	require.Contains(t, text, "#define SAMPCTL_VERSION \"1.2.3\"")
	require.Contains(t, text, "#define SAMPCTL_PLATFORM \"windows\"")
}

func TestBuildPrepareAllowsBuildFileDefaultsToBeOverridden(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()

	pcx := &PackageContext{
		PackageServices:       PackageServices{Platform: "linux"},
		PackageExecutionState: PackageExecutionState{AppVersion: "1.2.3"},
		Package: pawnpackage.Package{
			LocalPath: tempDir,
			Entry:     "gamemodes/test.pwn",
			Output:    "gamemodes/test.amx",
			Build: &build.Config{
				Constants: map[string]string{
					"SAMPCTL_VERSION":  "\"custom-version\"",
					"SAMPCTL_PLATFORM": "\"custom-platform\"",
				},
			},
		},
	}

	_, err := pcx.buildPrepare(context.Background(), "", false, false)
	require.NoError(t, err)

	buildFilePath := filepath.Join(tempDir, "sampctl_build_file.inc")
	contents, err := os.ReadFile(buildFilePath)
	require.NoError(t, err)

	text := string(contents)
	require.Contains(t, text, "#define SAMPCTL_VERSION \"custom-version\"")
	require.Contains(t, text, "#define SAMPCTL_PLATFORM \"custom-platform\"")
	require.NotContains(t, text, "#define SAMPCTL_VERSION \"1.2.3\"")
	require.NotContains(t, text, "#define SAMPCTL_PLATFORM \"linux\"")
}

func TestBuildPrepareBuildFileIncludesGitInfo(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	gitDir := filepath.Join(tempDir, ".git")
	require.NoError(t, os.MkdirAll(filepath.Join(gitDir, "refs", "heads"), 0o755))

	commit := "0123456789abcdef0123456789abcdef01234567"
	require.NoError(t, os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(gitDir, "refs", "heads", "main"), []byte(commit), 0o644))

	pcx := &PackageContext{
		Package: pawnpackage.Package{
			LocalPath: tempDir,
			Entry:     "gamemodes/test.pwn",
			Output:    "gamemodes/test.amx",
			Experimental: &pawnpackage.ExperimentalConfig{
				BuildFile: boolPtr(true),
			},
			Build: &build.Config{
				Constants: map[string]string{},
			},
		},
	}

	_, err := pcx.buildPrepare(context.Background(), "", false, false)
	require.NoError(t, err)

	buildFilePath := filepath.Join(tempDir, "sampctl_build_file.inc")
	contents, err := os.ReadFile(buildFilePath)
	require.NoError(t, err)

	text := string(contents)
	require.Contains(t, text, "#define SAMPCTL_BUILD_COMMIT \""+commit+"\"")
	require.Contains(t, text, "#define SAMPCTL_BUILD_COMMIT_SHORT \""+commit[:7]+"\"")
	require.Contains(t, text, "#define SAMPCTL_BUILD_BRANCH \"main\"")
}

func TestBuildPrepareSkipsBuildFileWhenExplicitlyDisabled(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()

	pcx := &PackageContext{
		Package: pawnpackage.Package{
			LocalPath: tempDir,
			Entry:     "gamemodes/test.pwn",
			Output:    "gamemodes/test.amx",
			Experimental: &pawnpackage.ExperimentalConfig{
				BuildFile: boolPtr(false),
			},
			Build: &build.Config{
				Constants: map[string]string{
					"DISABLED_CONST": "1",
				},
			},
		},
	}

	_, err := pcx.buildPrepare(context.Background(), "", false, false)
	require.NoError(t, err)

	buildFilePath := filepath.Join(tempDir, "sampctl_build_file.inc")
	_, err = os.Stat(buildFilePath)
	require.ErrorIs(t, err, os.ErrNotExist)
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

func TestShouldWatchBuildEvent(t *testing.T) {
	t.Parallel()

	require.True(t, shouldWatchBuildEvent(fsnotify.Event{Name: "gamemodes/test.pwn", Op: fsnotify.Write}))
	require.True(t, shouldWatchBuildEvent(fsnotify.Event{Name: "include/foo.inc", Op: fsnotify.Create}))
	require.False(t, shouldWatchBuildEvent(fsnotify.Event{Name: "README.md", Op: fsnotify.Write}))
	require.False(t, shouldWatchBuildEvent(fsnotify.Event{Name: "gamemodes/test.pwn", Op: fsnotify.Remove}))
}
