package commands

import (
	"errors"
	"flag"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/src/pkg/package/lockfile"
)

type fakeCommandLockfile struct {
	initVersion       string
	initErr           error
	saveErr           error
	lockfile          *lockfile.Lockfile
	hasLockfile       bool
	hasResolver       bool
	forceUpdateCalled bool
	buildRecord       struct {
		compilerVersion string
		compilerPreset  string
		entry           string
		output          string
	}
}

func (f *fakeCommandLockfile) InitLockfileResolver(sampctlVersion string) error {
	f.initVersion = sampctlVersion
	return f.initErr
}

func (f *fakeCommandLockfile) SaveLockfile() error {
	return f.saveErr
}

func (f *fakeCommandLockfile) HasLockfile() bool {
	return f.hasLockfile
}

func (f *fakeCommandLockfile) ForceUpdateLockfile() {
	f.forceUpdateCalled = true
}

func (f *fakeCommandLockfile) HasLockfileResolver() bool {
	return f.hasResolver
}

func (f *fakeCommandLockfile) GetLockfile() *lockfile.Lockfile {
	return f.lockfile
}

func (f *fakeCommandLockfile) RecordBuildToLockfile(compilerVersion, compilerPreset, entry, output string) {
	f.buildRecord.compilerVersion = compilerVersion
	f.buildRecord.compilerPreset = compilerPreset
	f.buildRecord.entry = entry
	f.buildRecord.output = output
}

func TestInitLockfileResolver(t *testing.T) {
	t.Parallel()

	target := &fakeCommandLockfile{}
	ctx := newTestCLIContext("dev")

	require.NoError(t, initLockfileResolver(ctx, target))
	assert.Equal(t, "dev", target.initVersion)

	target.initErr = errors.New("boom")
	err := initLockfileResolver(ctx, target)
	require.Error(t, err)
	assert.EqualError(t, err, "boom")
}

func TestDescribeEnsureLockfile(t *testing.T) {
	t.Parallel()

	withLockfile := &fakeCommandLockfile{hasLockfile: true}
	describeEnsureLockfile(withLockfile, true)
	assert.True(t, withLockfile.forceUpdateCalled)

	withoutLockfile := &fakeCommandLockfile{}
	describeEnsureLockfile(withoutLockfile, false)
	assert.False(t, withoutLockfile.forceUpdateCalled)
}

func TestRequireLockfileSupport(t *testing.T) {
	t.Parallel()

	require.NoError(t, requireLockfileSupport(&fakeCommandLockfile{hasResolver: true}))

	err := requireLockfileSupport(&fakeCommandLockfile{hasResolver: false})
	require.Error(t, err)
	assert.EqualError(t, err, "cannot use --lock-only without lockfile support")
}

func TestLockfileDependencyCount(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 0, lockfileDependencyCount(&fakeCommandLockfile{}))

	lf := lockfile.New("dev")
	lf.AddDependency("github.com/user/repo", lockfile.LockedDependency{Repo: "repo", User: "user"})
	lf.AddDependency("github.com/user/repo2", lockfile.LockedDependency{Repo: "repo2", User: "user"})
	assert.Equal(t, 2, lockfileDependencyCount(&fakeCommandLockfile{lockfile: lf}))
}

func TestSaveCommandLockfile(t *testing.T) {
	t.Parallel()

	require.NoError(t, saveCommandLockfile(&fakeCommandLockfile{}))

	err := saveCommandLockfile(&fakeCommandLockfile{saveErr: errors.New("save failed")})
	require.Error(t, err)
	assert.EqualError(t, err, "save failed")
}

func TestPersistBuildLockfile(t *testing.T) {
	t.Parallel()

	target := &fakeCommandLockfile{}
	require.NoError(t, persistBuildLockfile(target, "1.0.0", "default", "src/main.pwn", "gamemodes/test.amx"))
	assert.Equal(t, "1.0.0", target.buildRecord.compilerVersion)
	assert.Equal(t, "default", target.buildRecord.compilerPreset)
	assert.Equal(t, "src/main.pwn", target.buildRecord.entry)
	assert.Equal(t, "gamemodes/test.amx", target.buildRecord.output)
}

func newTestCLIContext(version string) *cli.Context {
	app := cli.NewApp()
	app.Metadata = map[string]interface{}{
		commandStateKey: newCommandState(version, "/tmp/cache"),
	}
	set := flag.NewFlagSet("test", flag.ContinueOnError)
	return cli.NewContext(app, set, nil)
}
