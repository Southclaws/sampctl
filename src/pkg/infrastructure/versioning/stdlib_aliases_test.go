package versioning

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestStdlibAliasesPreserveDependencyReferences(t *testing.T) {
	ResetDependencyOverrides()
	t.Cleanup(ResetDependencyOverrides)
	const commit = "1234567890123456789012345678901234567890"
	tests := []struct {
		format string
		want   DependencyMeta
	}{
		{format: "%s"},
		{format: "%s:0.3.7", want: DependencyMeta{Tag: "0.3.7"}},
		{format: "%s@open.mp", want: DependencyMeta{Branch: "open.mp"}},
		{format: "%s@feature/stdlib", want: DependencyMeta{Branch: "feature/stdlib"}},
		{format: "%s#" + commit, want: DependencyMeta{Commit: commit}},
		{format: "%s/includes/subdir@open.mp", want: DependencyMeta{Path: "includes/subdir", Branch: "open.mp"}},
		{format: "github.com/%s/includes:0.3.7", want: DependencyMeta{Path: "includes", Tag: "0.3.7"}},
		{format: "https://github.com/%s/includes@open.mp", want: DependencyMeta{Path: "includes", Branch: "open.mp"}},
		{format: "http://github.com/%s#" + commit, want: DependencyMeta{Commit: commit}},
		{format: "includes://%s/includes@open.mp", want: DependencyMeta{Scheme: "includes", Path: "includes", Branch: "open.mp"}},
		{format: "plugin://%s:0.3.7", want: DependencyMeta{Scheme: "plugin", Tag: "0.3.7"}},
		{format: "component://%s#" + commit, want: DependencyMeta{Scheme: "component", Commit: commit}},
		{format: "filterscript://%s@open.mp", want: DependencyMeta{Scheme: "filterscript", Branch: "open.mp"}},
		{format: "git@github.com:%s", want: DependencyMeta{SSH: "git"}},
		{format: "git@github.com:%s@open.mp", want: DependencyMeta{SSH: "git", Branch: "open.mp"}},
		{format: "git@github.com:%s/includes:0.3.7", want: DependencyMeta{SSH: "git", Path: "includes", Tag: "0.3.7"}},
		{format: "git@github.com:%s#" + commit, want: DependencyMeta{SSH: "git", Commit: commit}},
	}
	for _, alias := range []string{"Southclaws/samp-stdlib", "sampctl/samp-stdlib"} {
		for _, test := range tests {
			input := DependencyString(fmt.Sprintf(test.format, alias))
			t.Run(string(input), func(t *testing.T) {
				want := test.want
				want.Site, want.User, want.Repo = "github.com", "pawn-lang", "samp-stdlib"
				got, err := input.ExplodeWithOverrides(DependencyOverrides)
				if err != nil || got != want {
					t.Errorf("ExplodeWithOverrides(%q) = %+v, %v, want %+v, nil", input, got, err, want)
				}
				got, err = input.Explode()
				if err != nil || got != want {
					t.Errorf("Explode(%q) = %+v, %v, want %+v, nil", input, got, err, want)
				}
			})
		}
	}
}

func TestRepositoryOverridePreservesConfiguredReplacement(t *testing.T) {
	tests := []struct {
		input       DependencyString
		replacement string
		want        DependencyMeta
		wantErr     bool
	}{
		{
			input:       "https://github.com/Southclaws/samp-stdlib/includes@open.mp",
			replacement: "maintained/samp-stdlib:2.0",
			want:        DependencyMeta{Site: "github.com", User: "maintained", Repo: "samp-stdlib", Path: "includes", Tag: "2.0"},
		},
		{
			input:       "Southclaws/samp-stdlib@open.mp",
			replacement: "github.com/maintained/samp-stdlib:2.0",
			want:        DependencyMeta{Site: "github.com", User: "maintained", Repo: "samp-stdlib", Tag: "2.0"},
		},
		{
			input:       "Southclaws/samp-stdlib:1.0",
			replacement: "maintained/samp-stdlib@feature/stdlib",
			want:        DependencyMeta{Site: "github.com", User: "maintained", Repo: "samp-stdlib", Branch: "feature/stdlib"},
		},
		{
			input:       "includes://Southclaws/samp-stdlib@open.mp",
			replacement: "includes://local/includes",
			want:        DependencyMeta{Scheme: "includes", Local: "includes"},
		},
		{
			input:       "Southclaws/samp-stdlib@open.mp",
			replacement: "includes://local/includes",
			want:        DependencyMeta{Scheme: "includes", Local: "includes"},
		},
		{
			input:       "https://github.com/Southclaws/samp-stdlib@open.mp",
			replacement: "invalid replacement",
			wantErr:     true,
		},
	}
	for _, test := range tests {
		t.Run(test.replacement, func(t *testing.T) {
			got, err := test.input.ExplodeWithOverrides(map[string]string{"Southclaws/samp-stdlib": test.replacement})
			if (err != nil) != test.wantErr || got != test.want {
				t.Errorf("ExplodeWithOverrides(%q), replacement %q = %+v, %v, want %+v, error=%t", test.input, test.replacement, got, err, test.want, test.wantErr)
			}
		})
	}
}

func TestStdlibAliasesOnlyMatchTheirGitHubRepository(t *testing.T) {
	for _, input := range []string{
		"other/samp-stdlib",
		"OtherSouthclaws/samp-stdlib",
		"Southclaws/samp-stdlib-other",
		"Southclaws/samp-stdlib.inc",
		"Southclaws/samp-stdlibs",
		"sampctl/samp-stdlib-other",
		"https://gitlab.com/Southclaws/samp-stdlib@open.mp",
		"https://gitlab.com/sampctl/samp-stdlib/includes",
		"git@gitlab.com:Southclaws/samp-stdlib",
		"includes://local/Southclaws/samp-stdlib",
		"other/repo/includes/Southclaws/samp-stdlib",
	} {
		t.Run(input, func(t *testing.T) {
			if got := applyDependencyOverrides(input, DependencyOverrides); got != input {
				t.Errorf("applyDependencyOverrides(%q) = %q, want unchanged reference", input, got)
			}
		})
	}
}

func TestConfiguredStdlibAliasOverridesTakePrecedence(t *testing.T) {
	for _, key := range []string{"Southclaws/samp-stdlib", "github.com/Southclaws/samp-stdlib", "https://github.com/Southclaws/samp-stdlib"} {
		t.Run(key, func(t *testing.T) {
			useTemporaryDependencyOverridesConfig(t)
			configPath := DefaultDependencyOverridesPath()
			if err := SaveDependencyOverrides(map[string]string{key: "maintained/samp-stdlib:2.0"}, configPath); err != nil {
				t.Fatal(err)
			}
			ResetDependencyOverrides()
			t.Cleanup(ResetDependencyOverrides)
			local, err := LoadLocalDependencyOverrides(filepath.Dir(configPath))
			if err != nil {
				t.Fatal(err)
			}
			for _, reference := range []DependencyString{"Southclaws/samp-stdlib:1.0", "https://github.com/Southclaws/samp-stdlib@open.mp", "includes://Southclaws/samp-stdlib/includes@open.mp"} {
				want := DependencyMeta{Site: "github.com", User: "maintained", Repo: "samp-stdlib", Tag: "2.0"}
				if strings.HasPrefix(string(reference), "includes://") {
					want.Scheme, want.Path = "includes", "includes"
				}
				got, err := reference.Explode()
				if err != nil || got != want {
					t.Errorf("Explode(%q), override %q = %+v, %v, want %+v, nil", reference, key, got, err, want)
				}
				got, err = reference.ExplodeWithOverrides(local)
				if err != nil || got != want {
					t.Errorf("ExplodeWithOverrides(%q), override %q = %+v, %v, want %+v, nil", reference, key, got, err, want)
				}
			}
		})
	}
}
