package versioning

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProjectOverridesPrecedence(t *testing.T) {
	const original = "Zeex/samp-plugin-crashdetect"
	remote := map[string]string{original: "remote/crashdetect"}
	user := map[string]string{original: "user/crashdetect"}
	project := map[string]string{original: "project/crashdetect"}

	tests := []struct {
		name                  string
		remote, user, project map[string]string
		wantOwner             string
		wantProtocolOwner     string
	}{
		{name: "built-in default", wantOwner: "AmyrAhmady"},
		{name: "remote takes precedence over built-in", remote: remote, wantOwner: "remote"},
		{name: "user takes precedence over remote", remote: remote, user: user, wantOwner: "user"},
		{
			name:   "project takes precedence for exact declaration",
			remote: remote, user: user, project: project,
			wantOwner: "project", wantProtocolOwner: "user",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			remoteLoader := remoteOverridesLoader
			t.Cleanup(func() { remoteOverridesLoader = remoteLoader })
			remoteOverridesLoader = func(context.Context) map[string]string { return tc.remote }
			configPath := filepath.Join(t.TempDir(), "dependency-overrides.json")
			require.NoError(t, SaveDependencyOverrides(tc.user, configPath))
			legacy := LoadDependencyOverridesContext(t.Context(), configPath)
			policy, err := NewProjectOverrides(tc.project, "json")
			require.NoError(t, err)

			meta, err := policy.ResolveWithLegacy(original, legacy)

			require.NoError(t, err)
			assert.Equal(t, tc.wantOwner, meta.User)
			if tc.wantProtocolOwner != "" {
				meta, err = policy.ResolveWithLegacy("https://github.com/"+original, legacy)
				require.NoError(t, err)
				assert.Equal(t, tc.wantProtocolOwner, meta.User)
			}
		})
	}
}

func TestProjectOverridesExactAndSingleStep(t *testing.T) {
	policy, err := NewProjectOverrides(map[string]string{
		"fixture/PawnPlus:v1": "fixture/PawnPlus:v2",
		"fixture/PawnPlus:v2": "fixture/PawnPlus:v1",
		"fixture/PawnPlus:v3": "fixture/PawnPlus:v3",
	}, "yaml")
	require.NoError(t, err)

	for _, tc := range []struct {
		declaration DependencyString
		tag         string
	}{
		{"fixture/PawnPlus:v1", "v2"},
		{"fixture/PawnPlus:v2", "v1"},
		{"fixture/PawnPlus:v3", "v3"},
		{"fixture/PawnPlus:v4", "v4"},
		{"fixture/PawnPlus", ""},
		{"fixture/pawnplus:v1", "v1"},
		{"github.com/fixture/PawnPlus:v1", "v1"},
	} {
		t.Run(string(tc.declaration), func(t *testing.T) {
			// A legacy rule for the replacement must not run after the project substitution.
			meta, err := policy.ResolveWithLegacy(tc.declaration, map[string]string{"fixture/PawnPlus:v2": "fixture/PawnPlus:v5"})
			require.NoError(t, err)
			assert.Equal(t, tc.tag, meta.Tag)
		})
	}
}

func TestProjectOverridesValidation(t *testing.T) {
	for _, tc := range []struct {
		overrides map[string]string
		message   string
	}{
		{map[string]string{"invalid": "fixture/valid"}, `invalid key "invalid"`},
		{map[string]string{"fixture/valid": "invalid"}, `invalid replacement "invalid" for "fixture/valid"`},
		{map[string]string{"fixture/valid": ""}, `invalid replacement ""`},
		{map[string]string{"fixture/valid": "unknown://fixture/repo"}, "invalid replacement"},
		{map[string]string{"z-invalid": "", "a-invalid": ""}, `invalid key "a-invalid"`},
	} {
		_, err := NewProjectOverrides(tc.overrides, "yaml")
		require.ErrorContains(t, err, "pawn.yaml overrides")
		assert.ErrorContains(t, err, tc.message)
	}
}
