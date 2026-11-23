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
	contents := []byte("selected artifact")
	hash := sha256.Sum256(contents)
	for _, tc := range []struct {
		name       string
		makeOutput func(*testing.T, string)
		wantHash   string
	}{
		{
			name: "hashes the selected output",
			makeOutput: func(t *testing.T, path string) {
				require.NoError(t, os.WriteFile(path, contents, 0o644))
			},
			wantHash: "sha256:" + hex.EncodeToString(hash[:]),
		},
		{name: "leaves hash empty when the output is missing"},
		{
			name: "leaves hash empty when the output is a directory",
			makeOutput: func(t *testing.T, path string) {
				require.NoError(t, os.Mkdir(path, 0o755))
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			output := filepath.Join("build", "release.amx")
			path := filepath.Join(root, output)
			require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
			if tc.makeOutput != nil {
				tc.makeOutput(t, path)
			}
			resolver := &fakeDependencyLock{lockfile: lockfile.New("test")}
			pcx := &PackageContext{
				Package:              pawnpackage.Package{LocalPath: root},
				PackageLockfileState: PackageLockfileState{lockfileResolver: resolver},
			}

			pcx.RecordBuildToLockfile("version", "preset", "release.pwn", output)

			assert.Equal(t, lockfile.BuildRecord{
				CompilerVersion: "version",
				CompilerPreset:  "preset",
				Entry:           "release.pwn",
				Output:          output,
				OutputHash:      tc.wantHash,
			}, resolver.buildRecord)
		})
	}
}
