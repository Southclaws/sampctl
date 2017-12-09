package rook

import (
	"os"
	"testing"

	"github.com/Southclaws/sampctl/util"
	"github.com/Southclaws/sampctl/versioning"
)

func TestPackage_EnsureDependencies(t *testing.T) {
	os.MkdirAll("./tests/deps-sif", 0755) //nolint

	tests := []struct {
		name    string
		pkg     Package
		wantErr bool
	}{
		{"valid", Package{
			local: util.FullPath("./tests/deps-sif"),
			Dependencies: []versioning.DependencyString{
				"Southclaws/SIF:1.6.2",
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
