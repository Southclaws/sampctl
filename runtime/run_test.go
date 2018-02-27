package runtime

import (
	"bytes"
	"context"
	"path/filepath"
	"runtime"
	"testing"
	"time"

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
		{"bare", `----------
Loaded log file: "server_log.txt".
----------

SA-MP Dedicated Server
----------------------
v0.3.7-R2, (C)2005-2015 SA-MP Team


Server Plugins
--------------
	Loaded 0 plugins.


Started server on port: 7777, with maxplayers: 50 lanmode is OFF.


Filterscripts
---------------
	Loaded 0 filterscripts.

AllowAdminTeleport() : function is deprecated. Please see OnPlayerClickMap()

----------------------------------
	Bare Script

----------------------------------

Number of vehicle models: 0`,
			false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()

			config := types.MergeRuntimeDefault(&types.Runtime{})
			config.AppVersion = Version
			config.Platform = runtime.GOOS
			config.Version = "0.3.7"
			config.Gamemodes = []string{tt.name}
			config.WorkingDir = util.FullPath(filepath.Join("./tests/run/", tt.name))

			GetServerPackage("http://files.sa-mp.com", "0.3.7", config.WorkingDir, runtime.GOOS)

			if runtime.GOOS == "darwin" {
				config.Container = &types.ContainerConfig{
					MountCache: false,
				}
			}

			output := &bytes.Buffer{}
			err := Run(ctx, *config, util.FullPath("./tests/cache"), output, nil)
			assert.NoError(t, err)

			gotOutput := output.String()
			assert.Equal(t, tt.wantOutput, gotOutput)
		})
	}
}
