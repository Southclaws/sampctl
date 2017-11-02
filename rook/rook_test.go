package rook

import (
	"testing"

	"github.com/Southclaws/sampctl/util"
)

func TestPackage_EnsureDependencies(t *testing.T) {
	tests := []struct {
		name    string
		pkg     Package
		wantErr bool
	}{
		{"valid", Package{
			local: util.FullPath("./tests/deps-sif"),
			Dependencies: []Dependency{
				Dependency("Southclaws/SIF:1.6.2"),
			}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.pkg.EnsureDependencies(); (err != nil) != tt.wantErr {
				t.Errorf("Package.EnsureDependencies() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
