package pkgcontext

import (
	"context"
	"io"

	"github.com/google/go-github/github"

	runtimecfg "github.com/Southclaws/sampctl/src/pkg/runtime/run"
	runtimepkg "github.com/Southclaws/sampctl/src/pkg/runtime/runtime"
)

var _ RuntimeEnvironment = runtimeEnvironmentAdapter{}

type runtimeEnvironmentAdapter struct{}

func (runtimeEnvironmentAdapter) Run(ctx context.Context, cfg runtimecfg.Runtime, cacheDir string, passArgs, recover bool, output io.Writer, input io.Reader) error {
	return runtimepkg.Run(ctx, cfg, cacheDir, passArgs, recover, output, input)
}

func (runtimeEnvironmentAdapter) PrepareRuntimeDirectory(cacheDir, version, platform, scriptfiles string) error {
	return runtimepkg.PrepareRuntimeDirectory(cacheDir, version, platform, scriptfiles)
}

func (runtimeEnvironmentAdapter) CopyFileToRuntime(cacheDir, version, amxFile string) error {
	return runtimepkg.CopyFileToRuntime(cacheDir, version, amxFile)
}

func (runtimeEnvironmentAdapter) Ensure(ctx context.Context, gh *github.Client, cfg *runtimecfg.Runtime, noCache bool) error {
	return runtimepkg.Ensure(ctx, gh, cfg, noCache)
}

func (runtimeEnvironmentAdapter) GenerateConfig(cfg *runtimecfg.Runtime) error {
	return runtimepkg.GenerateConfig(cfg)
}
