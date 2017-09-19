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
			"valid",
			&Server{
				Announce:     "1",
				Hostname:     "Test",
				RCONPassword: "test",
				MaxPlayers:   "32",
				Port:         "7777",
				RCON:         "1",
				Language:     "English",
			},
			args{"./server.cfg"},
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
