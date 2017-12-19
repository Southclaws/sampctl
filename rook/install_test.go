package rook

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Southclaws/sampctl/versioning"
)

func TestPackage_Install(t *testing.T) {
	type args struct {
		target versioning.DependencyString
	}
	tests := []struct {
		name    string
		dir     string
		args    args
		wantErr bool
	}{
		{"simple", "./tests/install", args{"Southclaws/samp-ini"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pkg, err := PackageFromDir(true, tt.dir, "")
			if err != nil {
				t.Error(err)
			}

			err = Install(pkg, tt.args.target)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
