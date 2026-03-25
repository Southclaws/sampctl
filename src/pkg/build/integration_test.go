package build

import (
	"testing"
)

// TestOptionsEquivalentToArgs verifies that the new Options produce the same
// compiler arguments as the Args field
func TestOptionsEquivalentToArgs(t *testing.T) {
	boolPtr := func(b bool) *bool { return &b }
	intPtr := func(i int) *int { return &i }

	oldStyleConfig := Config{
		Name: "old-style",
		Args: []string{"-d3", "-;+", "-(+", "-\\+", "-Z+"},
	}

	newStyleConfig := Config{
		Name: "new-style",
		Options: &CompilerOptions{
			DebugLevel:             intPtr(3),
			RequireSemicolons:      boolPtr(true),
			RequireParentheses:     boolPtr(true),
			RequireEscapeSequences: boolPtr(true),
			CompatibilityMode:      boolPtr(true),
		},
	}

	newArgs := newStyleConfig.Options.ToArgs()

	if len(oldStyleConfig.Args) != len(newArgs) {
		t.Fatalf("Args length mismatch: old=%d, new=%d", len(oldStyleConfig.Args), len(newArgs))
	}

	for i := range oldStyleConfig.Args {
		if oldStyleConfig.Args[i] != newArgs[i] {
			t.Errorf("Arg[%d] mismatch: old=%q, new=%q", i, oldStyleConfig.Args[i], newArgs[i])
		}
	}
}

// TestDefaultProducesCorrectFlags verifies that Default() produces the expected
// SA:MP/open.mp standard flags
func TestDefaultProducesCorrectFlags(t *testing.T) {
	config := Default()
	args := config.Options.ToArgs()

	expectedArgs := []string{"-d3", "-;+", "-(+", "-\\+", "-Z+"}

	if len(args) != len(expectedArgs) {
		t.Fatalf("Default args length mismatch: got=%d, want=%d", len(args), len(expectedArgs))
	}

	for i := range expectedArgs {
		if args[i] != expectedArgs[i] {
			t.Errorf("Default arg[%d] mismatch: got=%q, want=%q", i, args[i], expectedArgs[i])
		}
	}
}

// TestConfigWithoutOptionsOrArgs ensures empty config doesn't crash
func TestConfigWithoutOptionsOrArgs(t *testing.T) {
	config := Config{
		Name: "empty",
	}

	// Should not panic
	if config.Options != nil {
		args := config.Options.ToArgs()
		if args != nil {
			t.Error("Expected nil args from nil Options")
		}
	}

	if config.Args != nil {
		t.Error("Expected nil Args in empty config")
	}
}
