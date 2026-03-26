package commands

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	transporthttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/src/config"
	"github.com/Southclaws/sampctl/src/pkg/package/pkgcontext"
)

func TestCommandStateConfigureBareSkipsConfigLoad(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	ctx := newGlobalCommandContext(t, []string{"--bare"})
	state := newCommandState("1.2.3", cacheDir)

	err := state.configure(ctx)
	require.NoError(t, err)
	assert.Nil(t, state.cfg)
	assert.Nil(t, state.gitAuth)
	assert.Nil(t, state.gh)
	assert.NoFileExists(t, filepath.Join(cacheDir, "config.json"))
}

func TestCommandStateConfigureLoadsConfigAndBuildsClients(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	require.NoError(t, config.WriteConfig(cacheDir, config.Config{
		DefaultUser: "alice",
		GitHubToken: "token",
	}))

	state := newCommandState("1.2.3", cacheDir)
	err := state.configure(newGlobalCommandContext(t, nil))
	require.NoError(t, err)

	require.NotNil(t, state.cfg)
	assert.Equal(t, "alice", state.cfg.DefaultUser)
	assert.Equal(t, "token", state.cfg.GitHubToken)
	assert.NotNil(t, state.gh)

	httpAuth := extractHTTPAuth(t, state.gitAuth)
	assert.Equal(t, &transporthttp.BasicAuth{Username: "x-access-token", Password: "token"}, httpAuth)
	require.NotNil(t, state.cfg.HideVersionUpdateMessage)
	assert.False(t, *state.cfg.HideVersionUpdateMessage)
}

func TestCommandStateConfigureWrapsLoadError(t *testing.T) {
	t.Parallel()

	cacheRoot := t.TempDir()
	cacheDir := filepath.Join(cacheRoot, "cache-file")
	require.NoError(t, os.WriteFile(cacheDir, []byte("not-a-directory"), 0o600))

	state := newCommandState("1.2.3", cacheDir)
	err := state.configure(newGlobalCommandContext(t, nil))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load or create sampctl config in")
	assert.Contains(t, err.Error(), cacheDir)
}

func TestCommandStateSaveConfigNoopWhenNil(t *testing.T) {
	t.Parallel()

	state := newCommandState("1.2.3", t.TempDir())
	require.NoError(t, state.saveConfig())
}

func TestCommandStateSaveConfigWritesConfig(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	hide := true
	state := newCommandState("1.2.3", cacheDir)
	state.cfg = &config.Config{
		DefaultUser:              "bob",
		HideVersionUpdateMessage: &hide,
	}

	require.NoError(t, state.saveConfig())

	loaded, err := config.LoadOrCreateConfig(cacheDir)
	require.NoError(t, err)
	assert.Equal(t, "bob", loaded.DefaultUser)
	require.NotNil(t, loaded.HideVersionUpdateMessage)
	assert.True(t, *loaded.HideVersionUpdateMessage)
}

func TestCommandStateSaveConfigWrapsWriteError(t *testing.T) {
	t.Parallel()

	cacheRoot := t.TempDir()
	cacheDir := filepath.Join(cacheRoot, "cache-file")
	require.NoError(t, os.WriteFile(cacheDir, []byte("not-a-directory"), 0o600))

	state := newCommandState("1.2.3", cacheDir)
	state.cfg = &config.Config{DefaultUser: "bob"}

	err := state.saveConfig()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to write updated configuration file to")
	assert.Contains(t, err.Error(), cacheDir)
}

func newGlobalCommandContext(t *testing.T, args []string) *cli.Context {
	t.Helper()

	app := cli.NewApp()
	flagSet := flag.NewFlagSet("test", flag.ContinueOnError)
	flagSet.Bool("bare", false, "")
	flagSet.Bool("verbose", false, "")
	require.NoError(t, flagSet.Parse(args))

	return cli.NewContext(app, flagSet, nil)
}

func extractHTTPAuth(t *testing.T, auth interface{}) *transporthttp.BasicAuth {
	t.Helper()

	switch value := auth.(type) {
	case *transporthttp.BasicAuth:
		return value
	case *pkgcontext.GitMultiAuth:
		require.IsType(t, &transporthttp.BasicAuth{}, value.HTTP)
		return value.HTTP.(*transporthttp.BasicAuth)
	default:
		t.Fatalf("unexpected auth type %T", auth)
		return nil
	}
}
