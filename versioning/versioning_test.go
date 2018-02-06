package versioning

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDependencyMeta_String(t *testing.T) {
	tests := []struct {
		name string
		meta DependencyMeta
		want string
	}{
		{"u/r", DependencyMeta{User: "user", Repo: "repo"}, "user/repo"},
		{"s/u/r", DependencyMeta{Site: "https://github.com", User: "user", Repo: "repo"}, "https://github.com/user/repo"},
		{"s/u/r:t", DependencyMeta{Site: "https://github.com", User: "user", Repo: "repo", Tag: "1.2.3"}, "https://github.com/user/repo:1.2.3"},
		{"s/u/r@b", DependencyMeta{Site: "https://github.com", User: "user", Repo: "repo", Branch: "dev"}, "https://github.com/user/repo@dev"},
		{"s/u/r#c", DependencyMeta{Site: "https://github.com", User: "user", Repo: "repo", Commit: "123abc"}, "https://github.com/user/repo#123abc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.meta.String(), tt.want)
		})
	}
}

func TestDependencyString_Explode(t *testing.T) {
	tests := []struct {
		name    string
		d       DependencyString
		wantDep DependencyMeta
		wantErr bool
	}{
		// Unversioned
		{"v u https url", DependencyString("https://github.com/user/repo.name"), DependencyMeta{Site: "https://github.com", User: "user", Repo: "repo.name"}, false},
		{"v u user/repo", DependencyString("user/repo.name"), DependencyMeta{Site: "https://github.com", User: "user", Repo: "repo.name"}, false},
		{"v u https url path", DependencyString("https://github.com/user/repo.name/inc/path"), DependencyMeta{Site: "https://github.com", User: "user", Repo: "repo.name", Path: "inc/path"}, false},
		{"v u user/repo path", DependencyString("user/repo.name/inc/path"), DependencyMeta{Site: "https://github.com", User: "user", Repo: "repo.name", Path: "inc/path"}, false},

		// Tag version
		{"v t https url", DependencyString("https://github.com/user/repo:1.2.3"), DependencyMeta{Site: "https://github.com", User: "user", Repo: "repo", Tag: "1.2.3"}, false},
		{"v t user/repo", DependencyString("user/repo:1.2.3"), DependencyMeta{Site: "https://github.com", User: "user", Repo: "repo", Tag: "1.2.3"}, false},
		{"v t user/repo", DependencyString("user/repo:^1.2.3"), DependencyMeta{Site: "https://github.com", User: "user", Repo: "repo", Tag: "^1.2.3"}, false},
		{"v t user/repo", DependencyString("user/repo:^2.0"), DependencyMeta{Site: "https://github.com", User: "user", Repo: "repo", Tag: "^2.0"}, false},
		{"v t user/repo", DependencyString("user/repo:2.1.x"), DependencyMeta{Site: "https://github.com", User: "user", Repo: "repo", Tag: "2.1.x"}, false},
		{"v t user/repo", DependencyString("user/repo:~1"), DependencyMeta{Site: "https://github.com", User: "user", Repo: "repo", Tag: "~1"}, false},
		{"v t user/repo", DependencyString("user/repo:~2.x"), DependencyMeta{Site: "https://github.com", User: "user", Repo: "repo", Tag: "~2.x"}, false},
		{"v t https url path", DependencyString("https://github.com/user/repo/inc/path:1.2.3"), DependencyMeta{Site: "https://github.com", User: "user", Repo: "repo", Path: "inc/path", Tag: "1.2.3"}, false},
		{"v t user/repo path", DependencyString("user/repo/inc/path:1.2.3"), DependencyMeta{Site: "https://github.com", User: "user", Repo: "repo", Path: "inc/path", Tag: "1.2.3"}, false},
		{"v t user/repo path", DependencyString("user/repo/inc/path:^1.2.3"), DependencyMeta{Site: "https://github.com", User: "user", Repo: "repo", Path: "inc/path", Tag: "^1.2.3"}, false},
		{"v t user/repo path", DependencyString("user/repo/inc/path:^2.0"), DependencyMeta{Site: "https://github.com", User: "user", Repo: "repo", Path: "inc/path", Tag: "^2.0"}, false},
		{"v t user/repo path", DependencyString("user/repo/inc/path:2.1.x"), DependencyMeta{Site: "https://github.com", User: "user", Repo: "repo", Path: "inc/path", Tag: "2.1.x"}, false},
		{"v t user/repo path", DependencyString("user/repo/inc/path:~1"), DependencyMeta{Site: "https://github.com", User: "user", Repo: "repo", Path: "inc/path", Tag: "~1"}, false},
		{"v t user/repo path", DependencyString("user/repo/inc/path:~2.x"), DependencyMeta{Site: "https://github.com", User: "user", Repo: "repo", Path: "inc/path", Tag: "~2.x"}, false},
		{"v t https url", DependencyString("https://github.com/user/repo:stable-release-3"), DependencyMeta{Site: "https://github.com", User: "user", Repo: "repo", Tag: "stable-release-3"}, false},
		{"v t user/repo", DependencyString("user/repo:stable-release-3"), DependencyMeta{Site: "https://github.com", User: "user", Repo: "repo", Tag: "stable-release-3"}, false},
		{"v t https url path", DependencyString("https://github.com/user/repo/inc/path:stable-release-3"), DependencyMeta{Site: "https://github.com", User: "user", Repo: "repo", Path: "inc/path", Tag: "stable-release-3"}, false},
		{"v t user/repo path", DependencyString("user/repo/inc/path:stable-release-3"), DependencyMeta{Site: "https://github.com", User: "user", Repo: "repo", Path: "inc/path", Tag: "stable-release-3"}, false},

		// Branch version
		{"v b https url", DependencyString("https://github.com/user/repo@dev"), DependencyMeta{Site: "https://github.com", User: "user", Repo: "repo", Branch: "dev"}, false},
		{"v b user/repo", DependencyString("user/repo@dev"), DependencyMeta{Site: "https://github.com", User: "user", Repo: "repo", Branch: "dev"}, false},
		{"v b https url path", DependencyString("https://github.com/user/repo/inc/path@dev"), DependencyMeta{Site: "https://github.com", User: "user", Repo: "repo", Path: "inc/path", Branch: "dev"}, false},
		{"v b user/repo path", DependencyString("user/repo/inc/path@dev"), DependencyMeta{Site: "https://github.com", User: "user", Repo: "repo", Path: "inc/path", Branch: "dev"}, false},

		// Commit hash version
		{"v c https url", DependencyString("https://github.com/user/repo#b96a2671133495950e0a0afe28f48ead48b06f1b"), DependencyMeta{Site: "https://github.com", User: "user", Repo: "repo", Commit: "b96a2671133495950e0a0afe28f48ead48b06f1b"}, false},
		{"v c user/repo", DependencyString("user/repo#b96a2671133495950e0a0afe28f48ead48b06f1b"), DependencyMeta{Site: "https://github.com", User: "user", Repo: "repo", Commit: "b96a2671133495950e0a0afe28f48ead48b06f1b"}, false},
		{"v c https url path", DependencyString("https://github.com/user/repo/inc/path#b96a2671133495950e0a0afe28f48ead48b06f1b"), DependencyMeta{Site: "https://github.com", User: "user", Repo: "repo", Path: "inc/path", Commit: "b96a2671133495950e0a0afe28f48ead48b06f1b"}, false},
		{"v c user/repo path", DependencyString("user/repo/inc/path#b96a2671133495950e0a0afe28f48ead48b06f1b"), DependencyMeta{Site: "https://github.com", User: "user", Repo: "repo", Path: "inc/path", Commit: "b96a2671133495950e0a0afe28f48ead48b06f1b"}, false},

		// Invalid
		{"i u www", DependencyString("www.github.com/user/repo"), DependencyMeta{}, true},
		{"i u user", DependencyString("http://github.com/repo"), DependencyMeta{}, true},
		{"i u project", DependencyString("project"), DependencyMeta{}, true},
		{"i u user:repo", DependencyString("user:repo"), DependencyMeta{}, true},
		{"i u naked url", DependencyString("github.com/user/repo.name"), DependencyMeta{}, true},
		{"i c naked url", DependencyString("github.com/user/repo.name#b96a2671133495950e0a0afe28f48ead48b06f1"), DependencyMeta{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotDep, err := tt.d.Explode()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tt.wantDep, gotDep)
		})
	}
}
