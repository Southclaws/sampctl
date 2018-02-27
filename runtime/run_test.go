package runtime

import (
	"testing"
)

func TestRun(t *testing.T) {
	tests := []struct {
		name       string
		wantOutput string
		wantErr    bool
	}{
		{"sampctl/samp-bare", `----------
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
			// ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			// defer cancel()

			// config := types.MergeRuntimeDefault(&types.Runtime{})

			// config.Platform = runtime.GOOS
			// config.Version = "0.3.7"

			// config.Gamemodes = []string{"bare"}
			// config.WorkingDir = GetRuntimePath(cacheDir, cfg.Version)

			// output := &bytes.Buffer{}
			// err = Run(ctx, config, "./tests/cache", output, nil)
			// assert.NoError(t, err)

			// if gotOutput := output.String(); gotOutput != tt.wantOutput {
			// 	t.Errorf("Run() = %v, want %v", gotOutput, tt.wantOutput)
			// }
		})
	}
}
