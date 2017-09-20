package main

import "testing"

func TestServer_Generate(t *testing.T) {
	type args struct {
		path string
	}
	tests := []struct {
		name    string
		server  *Server
		args    args
		wantErr bool
	}{
		{
			"required",
			&Server{
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
			args{"./testspace/server.cfg"},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.server.Generate(tt.args.path); (err != nil) != tt.wantErr {
				t.Errorf("Server.Generate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
