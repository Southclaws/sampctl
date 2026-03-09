package build

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetPredefinedCompilers(t *testing.T) {
	presets := GetPredefinedCompilers()
	require.Contains(t, presets, "samp")
	require.Contains(t, presets, "openmp")
	assert.Equal(t, "pawn-lang", presets["samp"].User)
	assert.Equal(t, "openmultiplayer", presets["openmp"].User)
}

func TestResolveCompilerConfig(t *testing.T) {
	t.Run("applies preset defaults", func(t *testing.T) {
		cfg := (&CompilerConfig{Preset: "samp"}).ResolveCompilerConfig()
		assert.Equal(t, "github.com", cfg.Site)
		assert.Equal(t, "pawn-lang", cfg.User)
		assert.Equal(t, "compiler", cfg.Repo)
		assert.Equal(t, "v3.10.10", cfg.Version)
	})

	t.Run("preserves explicit overrides", func(t *testing.T) {
		cfg := (&CompilerConfig{
			Preset:  "openmp",
			User:    "custom",
			Repo:    "fork",
			Version: "v9.9.9",
		}).ResolveCompilerConfig()
		assert.Equal(t, "github.com", cfg.Site)
		assert.Equal(t, "custom", cfg.User)
		assert.Equal(t, "fork", cfg.Repo)
		assert.Equal(t, "v9.9.9", cfg.Version)
	})

	t.Run("falls back without preset", func(t *testing.T) {
		cfg := (&CompilerConfig{}).ResolveCompilerConfig()
		assert.Equal(t, "github.com", cfg.Site)
		assert.Equal(t, "pawn-lang", cfg.User)
		assert.Equal(t, "compiler", cfg.Repo)
		assert.Equal(t, "v3.10.10", cfg.Version)
	})
}

func TestCompilerConfigHelpers(t *testing.T) {
	assert.Equal(t, "fallback", defaultString("", "fallback"))
	assert.Equal(t, "value", defaultString("value", "fallback"))
	assert.Equal(t, "v3.10.10", ensureCompilerVersion(""))
	assert.Equal(t, "v3.10.11", ensureCompilerVersion("3.10.11"))
	assert.Equal(t, "v3.10.11", ensureCompilerVersion("v3.10.11"))

	i := 99
	assert.Equal(t, []string{"-d3"}, intOption(&i, "-d", 0, 3))
	i = -1
	assert.Equal(t, []string{"-d0"}, intOption(&i, "-d", 0, 3))
	b := false
	assert.Nil(t, flagOption(&b, "-l"))
	b = true
	assert.Equal(t, []string{"-l"}, flagOption(&b, "-l"))
	b = false
	assert.Nil(t, boolOption(&b, "-x", ""))
	s := ""
	assert.Nil(t, stringOption(&s, "-e"))
}

func TestProblemHelpers(t *testing.T) {
	problems := Problems{
		{File: "a.pwn", Line: 1, Severity: ProblemWarning, Description: "warn"},
		{File: "b.pwn", Line: 2, Severity: ProblemError, Description: "err"},
		{File: "c.pwn", Line: 3, Severity: ProblemFatal, Description: "fatal"},
	}

	assert.Equal(t, "warning", ProblemWarning.String())
	assert.Equal(t, "error", ProblemError.String())
	assert.Equal(t, "fatal", ProblemFatal.String())
	assert.Equal(t, "unknown", ProblemSeverity(99).String())
	assert.Equal(t, "a.pwn:1 (warning) warn", problems[0].String())
	assert.Len(t, problems.Warnings(), 1)
	assert.Len(t, problems.Errors(), 1)
	assert.True(t, problems.Fatal())
	assert.False(t, problems.IsValid())
	assert.True(t, Problems{{Severity: ProblemWarning}}.IsValid())
}
