package runtime

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	textdistance "github.com/masatana/go-textdistance"
	"github.com/stretchr/testify/assert"

	"github.com/Southclaws/sampctl/types"
	"github.com/Southclaws/sampctl/util"
)

func TestRun(t *testing.T) {
	tests := []struct {
		name       string
		wantOutput string
		wantErr    bool
	}{
		{"bare", `
----------
Loaded log file: "server_log.txt".
----------

SA-MP Dedicated Server
----------------------
v0.3.7-R2, (C)2005-2015 SA-MP Team

plugins = ""  (string)

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
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
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

			err = Ensure(context.Background(), gh, &config, false, false)
			if err != nil {
				panic(err)
			}

			if runtime.GOOS == "darwin" {
				config.Container = &types.ContainerConfig{
					MountCache: false,
				}
			}

			output := &bytes.Buffer{}
			err = Run(ctx, config, util.FullPath("./tests/cache"), false, output, nil)
			if err != nil {
				if err.Error() != "received runtime error: failed to start server: exit status 1" {
					assert.NoError(t, err)
				}
			}

			gotOutput := output.String()
			distance := textdistance.LevenshteinDistance(gotOutput, tt.wantOutput)
			fmt.Println(distance)
			if distance > 150 {
				assert.Fail(t, "Output not similar enough", distance, 150)
				fmt.Println("\n%%", tt.wantOutput, "\n%%") // nolint
				fmt.Println("\n%%", gotOutput, "\n%%")     // nolint
			}
		})
	}
}
