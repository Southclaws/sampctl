package rook

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
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
				"pawn-lang/YSI-Includes",
				"samp-incognito/samp-streamer-plugin",
			}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotIncludes := FindIncludes(tt.args.files)
			assert.Equal(t, tt.wantIncludes, gotIncludes)
		})
	}
}

func TestFindIncludesMissingFileDoesNotHang(t *testing.T) {
	done := make(chan []versioning.DependencyString, 1)

	go func() {
		done <- FindIncludes([]string{"./tests/detect/does-not-exist.pwn"})
	}()

	select {
	case got := <-done:
		assert.Empty(t, got)
	case <-time.After(time.Second):
		t.Fatal("FindIncludes hung on missing file")
	}
}
