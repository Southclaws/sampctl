package pkgcontext

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/package/lockfile"
	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
)

func TestRecordBuildToLockfileResolvesProjectArtifact(t *testing.T) {
	for _, state := range []string{"file", "missing", "directory"} {
		t.Run(state, func(t *testing.T) {
			root := t.TempDir()
			output := filepath.Join("build", "release.amx")
			path := filepath.Join(root, output)
			require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
			contents := []byte("selected artifact")
			switch state {
			case "file":
				require.NoError(t, os.WriteFile(path, contents, 0o644))
			case "directory":
				require.NoError(t, os.Mkdir(path, 0o755))
			}
			resolver := &fakeDependencyLock{lockfile: lockfile.New("test")}
			pcx := &PackageContext{
				Package:              pawnpackage.Package{LocalPath: root},
				PackageLockfileState: PackageLockfileState{lockfileResolver: resolver},
			}
			pcx.RecordBuildToLockfile("version", "preset", "release.pwn", output)

			expectedHash := ""
			if state == "file" {
				hash := sha256.Sum256(contents)
				expectedHash = "sha256:" + hex.EncodeToString(hash[:])
			}
			assert.Equal(t, lockfile.BuildRecord{CompilerVersion: "version", CompilerPreset: "preset", Entry: "release.pwn", Output: output, OutputHash: expectedHash}, resolver.buildRecord)
		})
	}
}
