package pkgcontext

import (
	"context"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	runtimepkg "github.com/Southclaws/sampctl/src/pkg/runtime"
	runtimecfg "github.com/Southclaws/sampctl/src/pkg/runtime/config"
)

var _ RuntimeProvisioner = runtimeProvisionerAdapter{}

type runtimeProvisionerAdapter struct{}

func (runtimeProvisionerAdapter) EnsurePackageLayout(workingDir string, isOpenMP bool) error {
	return fs.EnsurePackageLayout(workingDir, isOpenMP)
}

func (runtimeProvisionerAdapter) EnsureBinaries(ctx context.Context, cacheDir string, cfg runtimecfg.Runtime) (*runtimepkg.RuntimeManifestInfo, error) {
	return runtimepkg.EnsureBinariesContext(ctx, cacheDir, cfg)
}

func (runtimeProvisionerAdapter) EnsurePlugins(request runtimepkg.EnsurePluginsRequest) error {
	return runtimepkg.EnsurePlugins(request)
}
