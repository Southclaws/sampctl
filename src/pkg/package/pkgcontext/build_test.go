package pkgcontext

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/build/build"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
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
