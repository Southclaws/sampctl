package types

import (
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRuntimeFromDir(t *testing.T) {
	type args struct {
		dir string
	}
	tests := []struct {
		name    string
		args    args
		wantCfg Runtime
		wantErr bool
	}{
		{"both basic", args{"./tests/load-both"}, Runtime{
			Format:       "json",
			Gamemodes:    []string{"rivershell"},
			RCONPassword: &[]string{"hello"}[0],
		}, false},
		{"both large", args{"./tests/load-yaml"}, Runtime{
			Format: "json",
			Gamemodes: []string{
				"rivershell",
				"baserace",
			},
			Plugins: []Plugin{
				"streamer",
				"zeex/samp-plugin-crashdetect",
			},
			RCONPassword: &[]string{"test"}[0],
			Port:         &[]int{8080}[0],
			Hostname:     &[]string{"Test"}[0],
			MaxPlayers:   &[]int{32}[0],
			Language:     &[]string{"English"}[0],
			Announce:     &[]bool{true}[0],
			RCON:         &[]bool{true}[0],
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := tt.args.dir
			tt.wantCfg.WorkingDir = dir

			tt.wantCfg.ToJSON()
			tt.wantCfg.ToYAML()

			gotCfg, err := RuntimeFromDir(tt.args.dir)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.wantCfg, gotCfg)
		})
	}
}
