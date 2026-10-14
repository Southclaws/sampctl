package pkgcontext

import (
	"archive/zip"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/package/resource"
)

func TestStagedResourcesKeepAdditionalFilesInsideOwnedDirectory(t *testing.T) {
	for _, destination := range []string{"absolute", "traversal", "relative"} {
		t.Run(destination, func(t *testing.T) {
			pcx, meta := reliabilityPackageContext(t)
			marker := filepath.Join(pcx.Package.Vendor, "marker")
			require.NoError(t, os.MkdirAll(pcx.Package.Vendor, 0o755))
			require.NoError(t, os.WriteFile(marker, []byte("previous"), 0o600))
			target := "extra/marker"
			switch destination {
			case "absolute":
				target = marker
			case "traversal":
				target = "../../../marker"
			}

			res := resource.Resource{
				Name: "^asset[.]zip$", Platform: "linux", Version: "0.3.7", Archive: true,
				Includes: []string{"^include[.]inc$"}, Files: map[string]string{"^extra[.]inc$": target},
			}
			definition, err := json.Marshal(map[string]any{"resources": []resource.Resource{res}})
			require.NoError(t, err)
			cachePath := meta.CachePath(pcx.CacheDir)
			seedReliabilityRepo(t, cachePath, string(definition))
			writeStagingResourceArchive(t, filepath.Join(cachePath, "asset.zip"))

			err = pcx.ensurePackage(context.Background(), meta, false)
			if destination == "relative" {
				require.NoError(t, err)
				require.FileExists(t, filepath.Join(pcx.Package.Vendor, res.Path(meta.Repo), target))
			} else {
				assert.ErrorIs(t, err, os.ErrInvalid)
				assert.NoDirExists(t, filepath.Join(pcx.Package.Vendor, meta.Repo))
			}

			contents, err := os.ReadFile(marker)
			require.NoError(t, err)
			require.Equal(t, "previous", string(contents))
			assertNoDependencyStages(t, pcx.Package.Vendor)
		})
	}
}

func writeStagingResourceArchive(t *testing.T, path string) {
	t.Helper()
	file, err := os.Create(path)
	require.NoError(t, err)

	archive := zip.NewWriter(file)
	for _, name := range []string{"include.inc", "extra.inc"} {
		writer, err := archive.Create(name)
		require.NoError(t, err)
		_, err = writer.Write([]byte("replacement"))
		require.NoError(t, err)
	}

	require.NoError(t, archive.Close())

	require.NoError(t, file.Close())
}
