package build

import (
	"reflect"
	"testing"
)

func TestCompilerOptions_ToArgs(t *testing.T) {
	boolPtr := func(b bool) *bool { return &b }
	intPtr := func(i int) *int { return &i }
	strPtr := func(s string) *string { return &s }

	tests := []struct {
		name string
		opts *CompilerOptions
		want []string
	}{
		{
			name: "nil options",
			opts: nil,
			want: nil,
		},
		{
			name: "empty options",
			opts: &CompilerOptions{},
			want: nil,
		},
		{
			name: "default SA:MP options",
			opts: &CompilerOptions{
				DebugLevel:             intPtr(3),
				RequireSemicolons:      boolPtr(true),
				RequireParentheses:     boolPtr(true),
				RequireEscapeSequences: boolPtr(true),
				CompatibilityMode:      boolPtr(true),
			},
			want: []string{"-d3", "-;+", "-(+", "-\\+", "-Z+"},
		},
		{
			name: "relaxed options",
			opts: &CompilerOptions{
				DebugLevel:             intPtr(0),
				RequireSemicolons:      boolPtr(false),
				RequireParentheses:     boolPtr(false),
				RequireEscapeSequences: boolPtr(false),
				CompatibilityMode:      boolPtr(false),
			},
			want: []string{"-d0", "-;-", "-(-", "-\\-", "-Z-"},
		},
		{
			name: "with optimization",
			opts: &CompilerOptions{
				DebugLevel:        intPtr(3),
				OptimizationLevel: intPtr(2),
			},
			want: []string{"-d3", "-O2"},
		},
		{
			name: "with listing and assembly",
			opts: &CompilerOptions{
				ShowListing:           boolPtr(true),
				ShowAnnotatedAssembly: boolPtr(true),
			},
			want: []string{"-l", "-a"},
		},
		{
			name: "with error file",
			opts: &CompilerOptions{
				ShowErrorFile: strPtr("errors.txt"),
			},
			want: []string{"-eerrors.txt"},
		},
		{
			name: "with warnings control",
			opts: &CompilerOptions{
				ShowWarnings: boolPtr(true),
			},
			want: []string{"-w+"},
		},
		{
			name: "disable warnings",
			opts: &CompilerOptions{
				ShowWarnings: boolPtr(false),
			},
			want: []string{"-w-"},
		},
		{
			name: "with compact encoding",
			opts: &CompilerOptions{
				CompactEncoding: boolPtr(true),
			},
			want: []string{"-C+"},
		},
		{
			name: "with tab size",
			opts: &CompilerOptions{
				TabSize: intPtr(4),
			},
			want: []string{"-t4"},
		},
		{
			name: "complex combination",
			opts: &CompilerOptions{
				DebugLevel:             intPtr(2),
				RequireSemicolons:      boolPtr(true),
				RequireParentheses:     boolPtr(true),
				RequireEscapeSequences: boolPtr(true),
				CompatibilityMode:      boolPtr(true),
				OptimizationLevel:      intPtr(1),
				ShowWarnings:           boolPtr(true),
				TabSize:                intPtr(4),
			},
			want: []string{"-d2", "-;+", "-(+", "-\\+", "-Z+", "-O1", "-w+", "-t4"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.opts.ToArgs()
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("CompilerOptions.ToArgs() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDefault(t *testing.T) {
	config := Default()

	if config == nil {
		t.Fatal("Default() returned nil")
	}

	if config.Options == nil {
		t.Fatal("Default() config has nil Options")
	}

	args := config.Options.ToArgs()
	expected := []string{"-d3", "-;+", "-(+", "-\\+", "-Z+"}

	if !reflect.DeepEqual(args, expected) {
		t.Errorf("Default() Options.ToArgs() = %v, want %v", args, expected)
	}

	if config.Compiler.Preset != "samp" {
		t.Errorf("Default() Compiler.Preset = %v, want samp", config.Compiler.Preset)
	}
}

func TestBackwardCompatibility(t *testing.T) {
	config := &Config{
		Args: []string{"-d3", "-;+", "-(+", "-\\+", "-Z+"},
	}

	if config.Options != nil {
		t.Error("Old-style config should not have Options set")
	}

	if len(config.Args) != 5 {
		t.Errorf("Expected 5 args, got %d", len(config.Args))
	}
}
