package commands

import (
	"bytes"
	"flag"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/google/go-github/github"
	"github.com/kirsle/configdir"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
	"github.com/Southclaws/sampctl/src/pkg/package/pkgcontext"
)

func TestDependencyDeclarationCommandOutput(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("HOME", configHome)
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("APPDATA", configHome)
	configdir.Refresh()
	t.Cleanup(configdir.Refresh)
	cacheDir, err := fs.ConfigDir()
	require.NoError(t, err)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		http.NotFound(w, nil)
	}))
	defer server.Close()
	baseURL, err := url.Parse(server.URL + "/")
	require.NoError(t, err)

	for _, development := range []bool{false, true} {
		for _, test := range []struct {
			name    string
			install bool
			targets []string
			left    []versioning.DependencyString
			changed bool
		}{
			{"install pinned bare", true, []string{"fixture/first"}, []versioning.DependencyString{"fixture/first:1.0.0", "fixture/second:1.0.0"}, false},
			{"install multiple existing", true, []string{"fixture/first", "fixture/second"}, []versioning.DependencyString{"fixture/first:1.0.0", "fixture/second:1.0.0"}, false},
			{"uninstall missing", false, []string{"fixture/missing"}, []versioning.DependencyString{"fixture/first:1.0.0", "fixture/second:1.0.0"}, false},
			{"uninstall unmatched pin", false, []string{"fixture/first:2.0.0"}, []versioning.DependencyString{"fixture/first:1.0.0", "fixture/second:1.0.0"}, false},
			{"uninstall one", false, []string{"fixture/first"}, []versioning.DependencyString{"fixture/second:1.0.0"}, true},
			{"uninstall multiple", false, []string{"fixture/first", "fixture/second"}, nil, true},
		} {
			name := test.name
			if development {
				name += " dev"
			}
			t.Run(name, func(t *testing.T) {
				dir := t.TempDir()
				pkg := pawnpackage.Package{Format: "json", LocalPath: dir, User: "fixture", Repo: "project"}
				selected := []versioning.DependencyString{"fixture/first:1.0.0", "fixture/second:1.0.0"}
				retained := []versioning.DependencyString{"fixture/retained:1.0.0"}
				pkg.Dependencies, pkg.Development = selected, retained
				if development {
					pkg.Dependencies, pkg.Development = retained, selected
				}
				require.NoError(t, pkg.WriteDefinition())
				before, err := os.ReadFile(filepath.Join(dir, "pawn.json"))
				require.NoError(t, err)
				state := newCommandState("dev", cacheDir)
				state.gh = github.NewClient(server.Client())
				state.gh.BaseURL = baseURL
				app := newCLIApp("dev", state)
				app.Writer, app.ErrWriter = io.Discard, io.Discard
				flags := packageUninstallFlags()
				command := packageUninstall
				if test.install {
					flags, command = packageInstallFlags(), packageInstall
				}
				set := flag.NewFlagSet(test.name, flag.ContinueOnError)
				for _, cliFlag := range withGlobalFlags(globalFlags(), flags) {
					cliFlag.Apply(set)
				}
				args := []string{"--dir", dir}
				if development {
					args = append(args, "--dev")
				}
				args = append(args, test.targets...)
				require.NoError(t, set.Parse(args))
				ctx := cli.NewContext(app, set, nil)
				output := captureStdout(t, func() {
					require.NoError(t, command(ctx))
				})
				message := "Dependency list unchanged"
				if test.changed {
					message = "Dependency list updated"
				}
				assert.Contains(t, output, message)
				updated, err := pawnpackage.PackageFromDir(dir)
				require.NoError(t, err)
				if development {
					assert.Equal(t, retained, updated.Dependencies)
					assert.Equal(t, test.left, updated.Development)
				} else {
					assert.Equal(t, test.left, updated.Dependencies)
					assert.Equal(t, retained, updated.Development)
				}
				if !test.changed {
					after, err := os.ReadFile(filepath.Join(dir, "pawn.json"))
					require.NoError(t, err)
					assert.Equal(t, before, after)
				}
				assert.Zero(t, requests.Load(), "these operations must not resolve remote dependencies")
			})
		}
	}
}

func TestPackageEnsureMissingSelector(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("HOME", configHome)
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("APPDATA", configHome)
	configdir.Refresh()
	t.Cleanup(configdir.Refresh)
	for _, format := range []string{"json", "yaml"} {
		t.Run(format, func(t *testing.T) {
			dir := t.TempDir()
			pkg := pawnpackage.Package{Format: format, LocalPath: dir, User: "fixture", Repo: "project"}
			require.NoError(t, pkg.WriteDefinition())
			path := filepath.Join(dir, "pawn."+format)
			before, err := os.ReadFile(path)
			require.NoError(t, err)
			ctx := newTestCLIContext("dev")
			set := flag.NewFlagSet("ensure", flag.ContinueOnError)
			for _, cliFlag := range withGlobalFlags(globalFlags(), packageEnsureFlags()) {
				cliFlag.Apply(set)
			}
			require.NoError(t, set.Parse([]string{"--dir", dir, "--update", "fixture/missing"}))
			ctx = cli.NewContext(ctx.App, set, nil)
			var output bytes.Buffer
			ctx.App.Writer = &output
			err = packageEnsure(ctx)
			assert.Equal(t, 1, ExitCode(err))
			var failure *pkgcontext.DependencyError
			require.ErrorAs(t, err, &failure)
			assert.Equal(t, "select", failure.Operation)
			assert.Equal(t, "fixture/missing", failure.Dependency)
			assert.Contains(t, output.String(), "fixture/missing")
			assert.Contains(t, output.String(), "Dependency is not declared in this project")
			assert.NotContains(t, output.String(), "Details:")
			after, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.Equal(t, before, after)
			assert.NoFileExists(t, filepath.Join(dir, "pawn.lock"))
		})
	}
}
