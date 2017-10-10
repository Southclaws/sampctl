package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_compilerURL(t *testing.T) {
	type args struct {
		filename string
		version  string
	}
	tests := []struct {
		name         string
		args         args
		wantURL      string
		wantFilename string
	}{
		{"v darwin", args{pawnMacOS, "3.10.2"}, "https://github.com/Zeex/pawn/releases/download/v3.10.2/pawnc-3.10.2-darwin.zip", "pawnc-3.10.2-darwin.zip"},
		{"v windows", args{pawnWin32, "3.10.2"}, "https://github.com/Zeex/pawn/releases/download/v3.10.2/pawnc-3.10.2-windows.zip", "pawnc-3.10.2-windows.zip"},
		{"v linux", args{pawnLinux, "3.10.2"}, "https://github.com/Zeex/pawn/releases/download/v3.10.2/pawnc-3.10.2-linux.tar.gz", "pawnc-3.10.2-linux.tar.gz"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotURL, gotFilename := compilerURL(tt.args.filename, tt.args.version)
			assert.Equal(t, tt.wantURL, gotURL)
			assert.Equal(t, tt.wantFilename, gotFilename)
		})
	}
}
