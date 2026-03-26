package pkgcontext

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
)

func TestDependencyUpdateRequestShouldForceDependency(t *testing.T) {
	t.Parallel()

	target := versioning.DependencyMeta{User: "u", Repo: "target"}

	tests := []struct {
		name    string
		request DependencyUpdateRequest
		meta    versioning.DependencyMeta
		direct  bool
		want    bool
	}{
		{
			name:    "disabled request does not force updates",
			request: DependencyUpdateRequest{},
			meta:    versioning.DependencyMeta{User: "u", Repo: "r", Tag: "latest"},
			direct:  true,
			want:    false,
		},
		{
			name: "dynamic direct dependency is refreshed during update",
			request: DependencyUpdateRequest{
				Enabled: true,
			},
			meta:   versioning.DependencyMeta{User: "u", Repo: "r", Tag: "latest"},
			direct: true,
			want:   true,
		},
		{
			name: "pinned dependency needs force",
			request: DependencyUpdateRequest{
				Enabled: true,
			},
			meta:   versioning.DependencyMeta{User: "u", Repo: "r", Tag: "1.0.0"},
			direct: true,
			want:   false,
		},
		{
			name: "force all refreshes transitive dependencies",
			request: DependencyUpdateRequest{
				Enabled: true,
				Force:   true,
			},
			meta:   versioning.DependencyMeta{User: "u", Repo: "r", Tag: "1.0.0"},
			direct: false,
			want:   true,
		},
		{
			name: "targeted force only refreshes the selected direct dependency",
			request: DependencyUpdateRequest{
				Enabled:    true,
				Force:      true,
				Target:     "u/target",
				TargetMeta: target,
			},
			meta:   versioning.DependencyMeta{User: "u", Repo: "other", Tag: "1.0.0"},
			direct: true,
			want:   false,
		},
		{
			name: "targeted force refreshes the selected dependency",
			request: DependencyUpdateRequest{
				Enabled:    true,
				Force:      true,
				Target:     "u/target",
				TargetMeta: target,
			},
			meta:   versioning.DependencyMeta{User: "u", Repo: "target", Tag: "1.0.0"},
			direct: true,
			want:   true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, tt.request.ShouldForceDependency(tt.meta, tt.direct))
		})
	}
}

func TestDependencyUpdateRequestMatches(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		request DependencyUpdateRequest
		meta    versioning.DependencyMeta
		want    bool
	}{
		{
			name:    "empty target matches any dependency",
			request: DependencyUpdateRequest{},
			meta:    versioning.DependencyMeta{User: "u", Repo: "r", Tag: "1.0.0"},
			want:    true,
		},
		{
			name: "bare target matches dependency regardless of version",
			request: DependencyUpdateRequest{
				Target:     "u/r",
				TargetMeta: versioning.DependencyMeta{User: "u", Repo: "r"},
			},
			meta: versioning.DependencyMeta{User: "u", Repo: "r", Tag: "2.0.0"},
			want: true,
		},
		{
			name: "versioned target only matches the exact version",
			request: DependencyUpdateRequest{
				Target:     "u/r:1.0.0",
				TargetMeta: versioning.DependencyMeta{User: "u", Repo: "r", Tag: "1.0.0"},
			},
			meta: versioning.DependencyMeta{User: "u", Repo: "r", Tag: "2.0.0"},
			want: false,
		},
		{
			name: "branch target does not match a tag selector",
			request: DependencyUpdateRequest{
				Target:     "u/r@main",
				TargetMeta: versioning.DependencyMeta{User: "u", Repo: "r", Branch: "main"},
			},
			meta: versioning.DependencyMeta{User: "u", Repo: "r", Tag: "1.0.0"},
			want: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, tt.request.Matches(tt.meta))
		})
	}
}
