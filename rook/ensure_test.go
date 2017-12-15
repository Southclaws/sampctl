package rook

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Southclaws/sampctl/util"
	"github.com/Southclaws/sampctl/versioning"
)

func TestMain(m *testing.M) {
	os.MkdirAll("./tests/deps", 0755)

	// Make sure our ensure tests dir is empty before running tests
	err := os.RemoveAll("./tests/deps")
	if err != nil {
		panic(err)
	}

	os.Exit(m.Run())
}

func TestPackage_EnsureDependencies(t *testing.T) {
	tests := []struct {
		name     string
		pkg      Package
		wantDeps []versioning.DependencyString
		wantErr  bool
	}{
		{"ensure", Package{
			local: util.FullPath("./tests/deps-ensure"),
			Dependencies: []versioning.DependencyString{
				"ScavengeSurvive/actions",
			}}, []versioning.DependencyString{
			"ScavengeSurvive/actions",
			"Southclaws/samp-stdlib",
			"Zeex/amx_assembly",
			"Misiur/YSI-Includes",
			"ScavengeSurvive/test-boilerplate",
			"ScavengeSurvive/velocity",
			"ScavengeSurvive/tick-difference",
		}, false},
	}
	for _, tt := range tests {
		os.MkdirAll(tt.pkg.local, 0755) //nolint

		t.Run(tt.name, func(t *testing.T) {
			err := tt.pkg.EnsureDependencies()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tt.wantDeps, tt.pkg.allDependencies)
		})
	}
}
