package rook

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/src-d/go-git.v4"
)

func TestMain(m *testing.M) {
	os.MkdirAll("./tests", 0755)

	// Make sure our test dir is empty before running tests
	err := os.RemoveAll("./tests/SIF*")
	if err != nil {
		panic(err)
	}

	os.Exit(m.Run())
}

func TestEnsurePackage(t *testing.T) {
	type args struct {
		vendorDirectory string
		pkg             Package
	}
	tests := []struct {
		name    string
		args    args
		wantSha string
		wantErr bool
		delete  bool
	}{
		{"SIF latest", args{"./tests", Package{
			user: "Southclaws",
			repo: "SIF",
		}}, "b1db5430428fe89f1cdbcb8267fe8f9f9b78df92", false, false},
		{"SIF 1.6.0", args{"./tests", Package{
			user:    "Southclaws",
			repo:    "SIF",
			version: "1.6.0",
		}}, "0693d8e85fd8b41a225912d26b2455449a6965a0", false, true},
		{"SIF 1.6.0", args{"./tests", Package{
			user:    "Southclaws",
			repo:    "SIF",
			version: "1.6.0",
		}}, "0693d8e85fd8b41a225912d26b2455449a6965a0", false, true},
		{"SIF 1.3.x", args{"./tests", Package{
			user:    "Southclaws",
			repo:    "SIF",
			version: "1.3.x",
		}}, "433fc17e9c6bf66bdf7ef3b82b70eea1c34af43f", false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := EnsurePackage(tt.args.vendorDirectory, tt.args.pkg)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			repo, _ := git.PlainOpen(filepath.Join("./tests", tt.args.pkg.repo)) //nolint
			ref, _ := repo.Head()
			assert.Equal(t, tt.wantSha, ref.Hash().String())

			// cleanup
			if tt.delete {
				err = os.RemoveAll(filepath.Join("./tests", tt.args.pkg.repo))
				if err != nil {
					panic(err)
				}
			}
		})
	}
}
