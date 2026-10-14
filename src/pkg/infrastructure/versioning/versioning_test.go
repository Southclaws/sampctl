package versioning

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/kirsle/configdir"
	"github.com/stretchr/testify/assert"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
)

func TestDependencyMeta_String(t *testing.T) {
	tests := []struct {
		name string
		meta DependencyMeta
		want string
	}{
		{"u/r", DependencyMeta{User: "user", Repo: "repo"}, "user/repo"},
		{"s/u/r", DependencyMeta{Site: "github.com", User: "user", Repo: "repo"}, "github.com/user/repo"},
		{"s/u/r:t", DependencyMeta{Site: "github.com", User: "user", Repo: "repo", Tag: "1.2.3"}, "github.com/user/repo:1.2.3"},
		{"s/u/r@b", DependencyMeta{Site: "github.com", User: "user", Repo: "repo", Branch: "dev"}, "github.com/user/repo@dev"},
		{"s/u/r#c", DependencyMeta{Site: "github.com", User: "user", Repo: "repo", Commit: "123abc"}, "github.com/user/repo#123abc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.meta.String())
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
		{"v u https url", DependencyString("https://github.com/user/repo.name"), DependencyMeta{Site: "github.com", User: "user", Repo: "repo.name"}, false},
		{"v u user/repo", DependencyString("user/repo.name"), DependencyMeta{Site: "github.com", User: "user", Repo: "repo.name"}, false},
		{"v u https url path", DependencyString("https://github.com/user/repo.name/inc/path"), DependencyMeta{Site: "github.com", User: "user", Repo: "repo.name", Path: "inc/path"}, false},
		{"v u user/repo path", DependencyString("user/repo.name/inc/path"), DependencyMeta{Site: "github.com", User: "user", Repo: "repo.name", Path: "inc/path"}, false},

		// Tag version
		{"v t https url", DependencyString("https://github.com/user/repo:1.2.3"), DependencyMeta{Site: "github.com", User: "user", Repo: "repo", Tag: "1.2.3"}, false},
		{"v t user/repo", DependencyString("user/repo:1.2.3"), DependencyMeta{Site: "github.com", User: "user", Repo: "repo", Tag: "1.2.3"}, false},
		{"v t user/repo", DependencyString("user/repo:^1.2.3"), DependencyMeta{Site: "github.com", User: "user", Repo: "repo", Tag: "^1.2.3"}, false},
		{"v t user/repo", DependencyString("user/repo:^2.0"), DependencyMeta{Site: "github.com", User: "user", Repo: "repo", Tag: "^2.0"}, false},
		{"v t user/repo", DependencyString("user/repo:2.1.x"), DependencyMeta{Site: "github.com", User: "user", Repo: "repo", Tag: "2.1.x"}, false},
		{"v t user/repo", DependencyString("user/repo:~1"), DependencyMeta{Site: "github.com", User: "user", Repo: "repo", Tag: "~1"}, false},
		{"v t user/repo", DependencyString("user/repo:~2.x"), DependencyMeta{Site: "github.com", User: "user", Repo: "repo", Tag: "~2.x"}, false},
		{"v t https url path", DependencyString("https://github.com/user/repo/inc/path:1.2.3"), DependencyMeta{Site: "github.com", User: "user", Repo: "repo", Path: "inc/path", Tag: "1.2.3"}, false},
		{"v t user/repo path", DependencyString("user/repo/inc/path:1.2.3"), DependencyMeta{Site: "github.com", User: "user", Repo: "repo", Path: "inc/path", Tag: "1.2.3"}, false},

		// Branch version
		{"v b https url", DependencyString("https://github.com/user/repo@branch"), DependencyMeta{Site: "github.com", User: "user", Repo: "repo", Branch: "branch"}, false},
		{"v b user/repo", DependencyString("user/repo@branch"), DependencyMeta{Site: "github.com", User: "user", Repo: "repo", Branch: "branch"}, false},
		{"v b https url path", DependencyString("https://github.com/user/repo/inc/path@branch"), DependencyMeta{Site: "github.com", User: "user", Repo: "repo", Path: "inc/path", Branch: "branch"}, false},
		{"v b user/repo path", DependencyString("user/repo/inc/path@branch"), DependencyMeta{Site: "github.com", User: "user", Repo: "repo", Path: "inc/path", Branch: "branch"}, false},

		// Commit version
		{"v c https url", DependencyString("https://github.com/user/repo#1234567890123456789012345678901234567890"), DependencyMeta{Site: "github.com", User: "user", Repo: "repo", Commit: "1234567890123456789012345678901234567890"}, false},
		{"v c user/repo", DependencyString("user/repo#1234567890123456789012345678901234567890"), DependencyMeta{Site: "github.com", User: "user", Repo: "repo", Commit: "1234567890123456789012345678901234567890"}, false},
		{"v c https url path", DependencyString("https://github.com/user/repo/inc/path#1234567890123456789012345678901234567890"), DependencyMeta{Site: "github.com", User: "user", Repo: "repo", Path: "inc/path", Commit: "1234567890123456789012345678901234567890"}, false},
		{"v c user/repo path", DependencyString("user/repo/inc/path#1234567890123456789012345678901234567890"), DependencyMeta{Site: "github.com", User: "user", Repo: "repo", Path: "inc/path", Commit: "1234567890123456789012345678901234567890"}, false},

		// Error cases
		{"invalid commit length", DependencyString("user/repo#123"), DependencyMeta{}, true},
		{"invalid version specifier", DependencyString("user/repo%invalid"), DependencyMeta{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dep, err := tt.d.Explode()
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.wantDep, dep)
		})
	}
}

func TestURLSchemeDependencies(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    DependencyMeta
		wantErr bool
	}{
		{
			name:  "plugin local",
			input: "plugin://local/plugins/mysql",
			want:  DependencyMeta{Scheme: "plugin", Local: "plugins/mysql"},
		},
		{
			name:  "component local",
			input: "component://local/components/pawn-memory",
			want:  DependencyMeta{Scheme: "component", Local: "components/pawn-memory"},
		},
		{
			name:  "includes local",
			input: "includes://local/legacy",
			want:  DependencyMeta{Scheme: "includes", Local: "legacy"},
		},
		{
			name:  "filterscript remote",
			input: "filterscript://southclaws/samp-object-loader",
			want:  DependencyMeta{Scheme: "filterscript", Site: "github.com", User: "southclaws", Repo: "samp-object-loader"},
		},
		{
			name:  "component remote with tag",
			input: "component://katursis/Pawn.RakNet:1.6.0-omp",
			want:  DependencyMeta{Scheme: "component", Site: "github.com", User: "katursis", Repo: "Pawn.RakNet", Tag: "1.6.0-omp"},
		},
		{
			name:  "filterscript remote with tag",
			input: "filterscript://southclaws/samp-object-loader:1.0.0",
			want:  DependencyMeta{Scheme: "filterscript", Site: "github.com", User: "southclaws", Repo: "samp-object-loader", Tag: "1.0.0"},
		},
		{
			name:    "invalid scheme",
			input:   "invalid://test",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dep, err := DependencyString(tt.input).Explode()
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.want, dep)
		})
	}
}

func TestURLSchemeString(t *testing.T) {
	tests := []struct {
		name string
		meta DependencyMeta
		want string
	}{
		{
			name: "plugin local",
			meta: DependencyMeta{Scheme: "plugin", Local: "plugins/mysql"},
			want: "plugin://local/plugins/mysql",
		},
		{
			name: "component local",
			meta: DependencyMeta{Scheme: "component", Local: "components/pawn-memory"},
			want: "component://local/components/pawn-memory",
		},
		{
			name: "includes local",
			meta: DependencyMeta{Scheme: "includes", Local: "legacy"},
			want: "includes://local/legacy",
		},
		{
			name: "filterscript remote",
			meta: DependencyMeta{Scheme: "filterscript", User: "southclaws", Repo: "samp-object-loader"},
			want: "filterscript://southclaws/samp-object-loader",
		},
		{
			name: "component remote with tag",
			meta: DependencyMeta{Scheme: "component", User: "katursis", Repo: "Pawn.RakNet", Tag: "1.6.0-omp"},
			want: "component://katursis/Pawn.RakNet:1.6.0-omp",
		},
		{
			name: "filterscript remote with tag",
			meta: DependencyMeta{Scheme: "filterscript", User: "southclaws", Repo: "samp-object-loader", Tag: "1.0.0"},
			want: "filterscript://southclaws/samp-object-loader:1.0.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.meta.String())
		})
	}
}

func TestLoadRemoteOverrides(t *testing.T) {
	useTemporaryDependencyOverridesConfig(t)

	err := ClearRemoteOverridesCache()
	assert.NoError(t, err)

	overrides := loadRemoteOverrides(context.Background())

	assert.NotNil(t, overrides)

	assert.IsType(t, map[string]string{}, overrides)
}

func TestLoadDependencyOverridesWithRemote(t *testing.T) {
	useTemporaryDependencyOverridesConfig(t)

	err := ClearRemoteOverridesCache()
	assert.NoError(t, err)

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test-overrides.json")

	localOverrides := map[string]string{
		"local/override": "local/replacement",
	}

	err = SaveDependencyOverrides(localOverrides, configPath)
	assert.NoError(t, err)

	overrides := LoadDependencyOverrides(configPath)

	assert.Equal(t, "AmyrAhmady/samp-plugin-crashdetect", overrides["Zeex/samp-plugin-crashdetect"])

	assert.Equal(t, "local/replacement", overrides["local/override"])
}

func TestClearRemoteOverridesCache(t *testing.T) {
	useTemporaryDependencyOverridesConfig(t)

	err := ClearRemoteOverridesCache()
	assert.NoError(t, err)

	cachePath := DefaultDependencyOverridesCachePath()
	dir := filepath.Dir(cachePath)
	err = os.MkdirAll(dir, 0o755)
	assert.NoError(t, err)

	err = os.WriteFile(cachePath, []byte(`{"overrides":{}}`), 0o644)
	assert.NoError(t, err)

	_, err = os.Stat(cachePath)
	assert.NoError(t, err)

	err = ClearRemoteOverridesCache()
	assert.NoError(t, err)

	_, err = os.Stat(cachePath)
	assert.True(t, os.IsNotExist(err))
}

func TestClearRemoteOverridesCacheUsesTemporaryConfigDirectory(t *testing.T) {
	t.Cleanup(configdir.Refresh)
	originalHome := t.TempDir()
	for _, name := range []string{"HOME", "XDG_CONFIG_HOME", "APPDATA"} {
		t.Setenv(name, originalHome)
	}
	configdir.Refresh()
	originalCacheDir := fs.ConfigDirPath()
	if rel, err := filepath.Rel(originalHome, originalCacheDir); err != nil || !filepath.IsLocal(rel) {
		t.Fatalf("ConfigDirPath() = %q, want path inside temporary home %q (relative path %q, error %v)", originalCacheDir, originalHome, rel, err)
	}

	originalPath := DefaultDependencyOverridesCachePath()
	originalContents := []byte(`{"overrides":{"original/repo":"preserved/repo"}}`)
	if err := os.WriteFile(originalPath, originalContents, 0o600); err != nil {
		t.Fatalf("WriteFile(%q) = %v, want nil", originalPath, err)
	}

	home := t.TempDir()
	for _, name := range []string{"HOME", "XDG_CONFIG_HOME", "APPDATA"} {
		t.Setenv(name, home)
	}
	configdir.Refresh()
	cacheDir := fs.ConfigDirPath()
	if rel, err := filepath.Rel(home, cacheDir); err != nil || !filepath.IsLocal(rel) {
		t.Fatalf("ConfigDirPath() = %q, want path inside temporary home %q (relative path %q, error %v)", cacheDir, home, rel, err)
	}

	cachePath := DefaultDependencyOverridesCachePath()
	if err := os.WriteFile(cachePath, []byte(`{"overrides":{}}`), 0o600); err != nil {
		t.Fatalf("WriteFile(%q) = %v, want nil", cachePath, err)
	}

	if err := ClearRemoteOverridesCache(); err != nil {
		t.Fatalf("ClearRemoteOverridesCache() = %v, want nil", err)
	}

	if _, err := os.Stat(cachePath); !os.IsNotExist(err) {
		t.Errorf("Stat(%q) = %v, want cache removed", cachePath, err)
	}

	got, err := os.ReadFile(originalPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) = %v, want original cache preserved", originalPath, err)
	}

	if !bytes.Equal(got, originalContents) {
		t.Errorf("ReadFile(%q) = %q, want %q", originalPath, got, originalContents)
	}
}
