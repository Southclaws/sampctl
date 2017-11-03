package rook

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/src-d/go-git.v4"
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
		{"SIF latest", args{"./tests/deps", Package{
			user: "Southclaws",
			repo: "SIF",
		}}, "b1db5430428fe89f1cdbcb8267fe8f9f9b78df92", false, false},
		{"SIF 1.3.x", args{"./tests/deps", Package{
			user:    "Southclaws",
			repo:    "SIF",
			version: "1.3.x",
		}}, "433fc17e9c6bf66bdf7ef3b82b70eea1c34af43f", false, true},
		{"SIF 1.4.x", args{"./tests/deps", Package{
			user:    "Southclaws",
			repo:    "SIF",
			version: "1.3.x",
		}}, "706daf942e2aa4c2460ecacb459c354ba6951fd0", false, false},
		// {"crashdetect latest", args{"./tests/deps", Package{
		// 	user: "Zeex",
		// 	repo: "samp-plugin-crashdetect",
		// }}, "722f3e80e47b74ff694fd1805b9d2922c2c15ce0", false, true},
		// {"crashdetect 4.15", args{"./tests/deps", Package{
		// 	user:    "Zeex",
		// 	repo:    "samp-plugin-crashdetect",
		// 	version: "4.15.x",
		// }}, "b4d4abd02bfc7052a73af27ca5d828c8ec057481", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := EnsurePackage(tt.args.vendorDirectory, tt.args.pkg)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			repo, _ := git.PlainOpen(filepath.Join("./tests/deps", tt.args.pkg.repo)) //nolint
			ref, _ := repo.Head()
			assert.Equal(t, tt.wantSha, ref.Hash().String())

			// cleanup
			if tt.delete {
				err = os.RemoveAll(filepath.Join("./tests/deps", tt.args.pkg.repo))
				if err != nil {
					panic(err)
				}
			}
		})
	}
}
