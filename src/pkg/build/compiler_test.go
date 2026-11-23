package build

import (
	"reflect"
	"testing"
)

func TestCompilerOptions_ToArgs(t *testing.T) {
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
				DebugLevel:             new(3),
				RequireSemicolons:      new(true),
				RequireParentheses:     new(true),
				RequireEscapeSequences: new(true),
				CompatibilityMode:      new(true),
			},
			want: []string{"-d3", "-;+", "-(+", "-\\+", "-Z+"},
		},
		{
			name: "relaxed options",
			opts: &CompilerOptions{
				DebugLevel:             new(0),
				RequireSemicolons:      new(false),
				RequireParentheses:     new(false),
				RequireEscapeSequences: new(false),
				CompatibilityMode:      new(false),
			},
			want: []string{"-d0", "-;-", "-(-", "-\\-", "-Z-"},
		},
		{
			name: "with optimization",
			opts: &CompilerOptions{
				DebugLevel:        new(3),
				OptimizationLevel: new(2),
			},
			want: []string{"-d3", "-O2"},
		},
		{
			name: "with listing and assembly",
			opts: &CompilerOptions{
				ShowListing:           new(true),
				ShowAnnotatedAssembly: new(true),
			},
			want: []string{"-l", "-a"},
		},
		{
			name: "with error file",
			opts: &CompilerOptions{
				ShowErrorFile: new("errors.txt"),
			},
			want: []string{"-eerrors.txt"},
		},
		{
			name: "enable warnings",
			opts: &CompilerOptions{
				ShowWarnings: new(true),
			},
			want: []string{"-w+"},
		},
		{
			name: "disable warnings",
			opts: &CompilerOptions{
				ShowWarnings: new(false),
			},
			want: []string{"-w-"},
		},
		{
			name: "with compact encoding",
			opts: &CompilerOptions{
				CompactEncoding: new(true),
			},
			want: []string{"-C+"},
		},
		{
			name: "with tab size",
			opts: &CompilerOptions{
				TabSize: new(4),
			},
			want: []string{"-t4"},
		},
		{
			name: "combined options preserve argument order",
			opts: &CompilerOptions{
				DebugLevel:             new(2),
				RequireSemicolons:      new(true),
				RequireParentheses:     new(true),
				RequireEscapeSequences: new(true),
				CompatibilityMode:      new(true),
				OptimizationLevel:      new(1),
				ShowWarnings:           new(true),
				TabSize:                new(4),
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

func TestCompilerConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     CompilerConfig
		wantErr bool
	}{
		{
			name:    "remote only",
			cfg:     CompilerConfig{Preset: "samp"},
			wantErr: false,
		},
		{
			name:    "local only",
			cfg:     CompilerConfig{Path: "/tmp/custom-compiler"},
			wantErr: false,
		},
		{
			name:    "path with preset",
			cfg:     CompilerConfig{Path: "/tmp/custom-compiler", Preset: "samp"},
			wantErr: true,
		},
		{
			name:    "path with repo",
			cfg:     CompilerConfig{Path: "/tmp/custom-compiler", Repo: "compiler"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error but got nil")
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
