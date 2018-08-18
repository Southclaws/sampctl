package runtime

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/Southclaws/sampctl/types"
	"github.com/Southclaws/sampctl/util"
)

func TestRun(t *testing.T) {
	// these tests don't run on mac!
	if runtime.GOOS == "darwin" {
		return
	}

	tests := []struct {
		name       string
		wantOutput string
		wantErr    bool
	}{
		{"bare", `
Server Plugins
--------------
 Loaded 0 plugins.


Started server on port: 7777, with maxplayers: 32 lanmode is OFF.


Filterscripts
---------------
  Loaded 0 filterscripts.

AllowAdminTeleport() : function is deprecated. Please see OnPlayerClickMap()

----------------------------------
  Bare Script

----------------------------------

Number of vehicle models: 0
`,
			false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
			defer cancel()

			dir := util.FullPath(filepath.Join("./tests/run/", tt.name))

			config, err := NewConfigFromEnvironment(dir)
			if err != nil {
				panic(err)
			}
			config.AppVersion = Version
			config.Platform = runtime.GOOS
			config.Version = "0.3.7"
			config.Gamemodes = []string{tt.name}
			config.WorkingDir = dir

			err = Ensure(context.Background(), gh, &config, false)
			if err != nil {
				panic(err)
			}

			if runtime.GOOS == "darwin" {
				config.Container = &types.ContainerConfig{
					MountCache: false,
				}
			}

			output := &bytes.Buffer{}
			input := &bytes.Buffer{}

			err = Run(ctx, config, util.FullPath("./tests/cache"), false, false, output, input) //nolint
			if err != nil {
				if err.Error() != "received runtime error: failed to start server: failed to start pty: context deadline exceeded" {
					assert.NoError(t, err)
				}
			}

			gotOutput := output.String()

			if !strings.HasSuffix(gotOutput, tt.wantOutput) {
				assert.Fail(t, fmt.Sprintf("Output not similar enough:\n---Expected:\n%s---Actual:\n%s", tt.wantOutput, gotOutput))
			}
		})
	}
}
