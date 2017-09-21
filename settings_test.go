package main

import "testing"

func TestServer_GenerateServerCfg(t *testing.T) {
	type args struct {
		path string
	}
	tests := []struct {
		name    string
		cfg     *Config
		args    args
		wantErr bool
	}{
		{
			"required",
			&Config{
				// Announce:     &[]bool{true}[0],
				// Hostname:     &[]string{"Test"}[0],
				// MaxPlayers:   &[]int{32}[0],
				// Port:         &[]int{8080}[0],
				// RCON:         &[]bool{true}[0],
				// Language:     &[]string{"English"}[0],
				Gamemode: &[]string{
					"rivershell",
					"baserace",
				},
				RCONPassword: &[]string{"test"}[0],
			},
			args{"./testspace"},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.cfg.GenerateServerCfg(tt.args.path); (err != nil) != tt.wantErr {
				t.Errorf("Config.GenerateServerCfg() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
