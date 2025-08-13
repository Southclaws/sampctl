package rook

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Southclaws/sampctl/src/pkg/package/pkgcontext"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
)

func Test_PackageInstall(t *testing.T) {
	type args struct {
		targets     []versioning.DependencyString
		development bool
	}
	tests := []struct {
		name    string
		pkg     []byte
		args    args
		wantErr bool
	}{
		{"simple", []byte(`{
			"user": "Southclaws",
			"repo": "install-test",
			"entry": "gamemodes/test.pwn",
			"output": "gamemodes/test.amx",
			"dependencies": ["pawn-lang/samp-stdlib"]
		}`), args{[]versioning.DependencyString{"thecodeah/pawn-humanize:v1.1.1"}, false}, false},
		{"dev", []byte(`{
			"user": "Southclaws",
			"repo": "install-test",
			"entry": "gamemodes/test.pwn",
			"output": "gamemodes/test.amx",
			"dependencies": ["pawn-lang/samp-stdlib"]
		}`), args{[]versioning.DependencyString{"thecodeah/pawn-humanize:v1.1.1"}, true}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := filepath.Join("./tests/install", tt.name)
			_ = os.RemoveAll(dir)
			_ = os.MkdirAll(dir, 0700)

			ioutil.WriteFile(filepath.Join(dir, "pawn.json"), tt.pkg, 0700) // nolint

			pcx1, err := pkgcontext.NewPackageContext(gh, gitAuth, true, dir, runtime.GOOS, "./tests/cache", "", false)
			if err != nil {
				t.Error(err)
			}

			err = pcx1.Install(context.Background(), tt.args.targets, tt.args.development)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			pcx2, err := pkgcontext.NewPackageContext(gh, gitAuth, true, dir, runtime.GOOS, "./tests/cache", "", false)
			if err != nil {
				t.Error(err)
			}

			if tt.args.development {
				for _, target := range tt.args.targets {
					assert.Contains(t, pcx2.Package.Development, target)
				}
			} else {
				for _, target := range tt.args.targets {
					assert.Contains(t, pcx2.Package.Dependencies, target)
				}
			}
		})
	}
}

func Test_PackageGet(t *testing.T) {
	type args struct {
		dep versioning.DependencyMeta
		dir string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{"direct", args{versioning.DependencyMeta{Site: "github.com", User: "thecodeah", Repo: "pawn-humanize", Tag: "v1.1.1"}, "./tests/get/direct"}, false},
		{"get-auto", args{versioning.DependencyMeta{Site: "github.com", User: "thecodeah", Repo: "pawn-humanize", Tag: "v1.1.1"}, "./tests/get"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.args.dir == "./tests/get" {
				if os.RemoveAll(filepath.Join(tt.args.dir, tt.args.dep.Repo)) != nil {
					panic("failed to remove get test dir")
				}
			} else {
				if os.RemoveAll(tt.args.dir) != nil {
					panic("failed to remove get test dir")
				}
			}

			err := Get(context.Background(), gh, tt.args.dep, tt.args.dir, nil, runtime.GOOS, "./tests/cache")
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
