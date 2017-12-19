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
		wantDeps []versioning.DependencyMeta
		wantErr  bool
	}{
		{"ensure", Package{
			Local: util.FullPath("./tests/deps-ensure"),
			Dependencies: []versioning.DependencyString{
				"ScavengeSurvive/actions",
			}},
			[]versioning.DependencyMeta{
				versioning.DependencyMeta{User: "ScavengeSurvive", Repo: "actions", Path: "", Version: ""},
				versioning.DependencyMeta{User: "Southclaws", Repo: "samp-stdlib", Path: "", Version: ""},
				versioning.DependencyMeta{User: "ScavengeSurvive", Repo: "test-boilerplate", Path: "", Version: ""},
				versioning.DependencyMeta{User: "Zeex", Repo: "amx_assembly", Path: "", Version: ""},
				versioning.DependencyMeta{User: "Misiur", Repo: "YSI-Includes", Path: "", Version: ""},
				versioning.DependencyMeta{User: "ScavengeSurvive", Repo: "velocity", Path: "", Version: ""},
				versioning.DependencyMeta{User: "ScavengeSurvive", Repo: "tick-difference", Path: "", Version: ""},
			}, false},
	}
	for _, tt := range tests {
		os.MkdirAll(tt.pkg.Local, 0755) //nolint

		t.Run(tt.name, func(t *testing.T) {
			err := tt.pkg.EnsureDependencies()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tt.wantDeps, tt.pkg.AllDependencies)
		})
	}
}
