package rook

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Southclaws/sampctl/versioning"
)

func TestFindIncludes(t *testing.T) {
	type args struct {
		files []string
	}
	tests := []struct {
		name         string
		args         args
		wantIncludes []versioning.DependencyString
	}{
		{"one-file", args{[]string{"./tests/detect/test1.pwn"}},
			[]versioning.DependencyString{
				"samp-incognito/samp-streamer-plugin",
				"pawn-lang/YSI-Includes",
			}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotIncludes := FindIncludes(tt.args.files)
			assert.Equal(t, tt.wantIncludes, gotIncludes)
		})
	}
}
