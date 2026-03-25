package pkgcontext

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-github/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	runtimecfg "github.com/Southclaws/sampctl/src/pkg/runtime/run"
	runtimepkg "github.com/Southclaws/sampctl/src/pkg/runtime/runtime"
)

type fakeRuntimeEnvironment struct {
	runCalled      bool
	prepareCalled  bool
	copyCalled     bool
	ensureCalled   bool
	generateCalled bool
	lastWorkingDir string
	lastCacheDir   string
	lastBinaryPath string
}

var _ RuntimeEnvironment = (*fakeRuntimeEnvironment)(nil)

func (f *fakeRuntimeEnvironment) Run(context.Context, runtimecfg.Runtime, runtimepkg.RunOptions) error {
	f.runCalled = true
	return nil
}

func (f *fakeRuntimeEnvironment) PrepareRuntimeDirectory(cacheDir, version, platform, scriptfiles string) error {
	f.prepareCalled = true
	f.lastCacheDir = cacheDir
	return nil
}

func (f *fakeRuntimeEnvironment) CopyFileToRuntime(cacheDir, version, amxFile string) error {
	f.copyCalled = true
	f.lastCacheDir = cacheDir
	f.lastBinaryPath = amxFile
	return nil
}

func (f *fakeRuntimeEnvironment) Ensure(context.Context, *github.Client, *runtimecfg.Runtime, bool) error {
	f.ensureCalled = true
	return nil
}

func (f *fakeRuntimeEnvironment) GenerateConfig(cfg *runtimecfg.Runtime) error {
	f.generateCalled = true
	f.lastWorkingDir = cfg.WorkingDir
	return nil
}

func TestRunPrepareUsesInjectedRuntimeEnvironment(t *testing.T) {
	t.Parallel()

	projectDir := t.TempDir()
	outputPath := filepath.Join(projectDir, "gamemodes", "main.amx")
	require.NoError(t, os.MkdirAll(filepath.Dir(outputPath), 0o755))
	require.NoError(t, os.WriteFile(outputPath, []byte("amx"), 0o644))

	local := false
	config := map[string]any{
		"entry":  "gamemodes/main.pwn",
		"output": "gamemodes/main.amx",
		"local":  local,
		"runtime": map[string]any{
			"version": "0.3.7",
		},
	}
	data, err := json.Marshal(config)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "pawn.json"), data, 0o644))

	fakeEnv := &fakeRuntimeEnvironment{}
	pcx, err := NewPackageContext(NewPackageContextOptions{
		Parent:   true,
		Dir:      projectDir,
		Platform: "linux",
		CacheDir: t.TempDir(),
	})
	require.NoError(t, err)
	pcx.RuntimeEnv = fakeEnv

	err = pcx.RunPrepare(context.Background())
	require.NoError(t, err)
	assert.True(t, fakeEnv.prepareCalled)
	assert.True(t, fakeEnv.copyCalled)
	assert.True(t, fakeEnv.ensureCalled)
	assert.True(t, fakeEnv.generateCalled)
	assert.Equal(t, outputPath, fakeEnv.lastBinaryPath)
	assert.Equal(t, filepath.Join(pcx.CacheDir, "runtime", "0.3.7"), fakeEnv.lastWorkingDir)
}
