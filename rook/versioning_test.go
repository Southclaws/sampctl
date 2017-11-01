package rook

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDependency_Validate(t *testing.T) {
	tests := []struct {
		name      string
		d         Dependency
		wantValid bool
		wantErr   bool
	}{
		// Unversioned
		{"v u https url", Dependency("https://github.com/user/repo"), true, false},
		{"v u http url", Dependency("http://github.com/user/repo"), true, false},
		{"v u naked url", Dependency("github.com/user/repo"), true, false},
		{"v u user/repo", Dependency("user/repo"), true, false},

		// Versioned - semver
		{"v v https url", Dependency("https://github.com/user/repo:1.2.3"), true, false},
		{"v v http url", Dependency("http://github.com/user/repo:1.2.3"), true, false},
		{"v v naked url", Dependency("github.com/user/repo:1.2.3"), true, false},
		{"v v user/repo", Dependency("user/repo:1.2.3"), true, false},
		{"v v user/repo", Dependency("user/repo:^1.2.3"), true, false},
		{"v v user/repo", Dependency("user/repo:^2.0"), true, false},
		{"v v user/repo", Dependency("user/repo:2.1.x"), true, false},
		{"v v user/repo", Dependency("user/repo:~1"), true, false},
		{"v v user/repo", Dependency("user/repo:~2.x"), true, false},

		// Versioned - custom
		{"v c https url", Dependency("https://github.com/user/repo:stable-release-3"), true, true},
		{"v c http url", Dependency("http://github.com/user/repo:stable-release-3"), true, true},
		{"v c naked url", Dependency("github.com/user/repo:stable-release-3"), true, true},
		{"v c user/repo", Dependency("user/repo:stable-release-3"), true, true},

		// Unversioned - Invalid
		{"i u www", Dependency("www.github.com/user/repo"), false, true},
		{"i u user", Dependency("http://github.com/repo"), false, true},
		{"i u project", Dependency("project"), false, true},
		{"i u user:repo", Dependency("user:repo"), false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotValid, err := tt.d.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tt.wantValid, gotValid)
		})
	}
}
