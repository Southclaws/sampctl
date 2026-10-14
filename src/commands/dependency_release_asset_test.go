package commands

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"strings"
	"testing"

	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/download"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/pkgcontext"
)

func TestReportMissingPluginReleaseAsset(t *testing.T) {
	t.Parallel()

	for _, verbose := range []bool{false, true} {
		t.Run(fmt.Sprintf("verbose=%t", verbose), func(t *testing.T) {
			ctx := newTestCLIContext("dev")
			set := flag.NewFlagSet("test", flag.ContinueOnError)
			set.Bool("verbose", verbose, "")
			ctx = cli.NewContext(ctx.App, set, nil)
			var output bytes.Buffer
			ctx.App.Writer = &output

			missing := &download.ReleaseAssetNotFoundError{
				Meta:            versioning.DependencyMeta{User: "Southclaws", Repo: "pawn-yaml", Tag: "latest"},
				Tag:             "1.0.0",
				Pattern:         "yaml.so",
				AvailableAssets: []string{"yaml.dll"},
			}
			failure := &pkgcontext.OperationError{
				Operation: "dependency operation",
				Outcome:   pkgcontext.OutcomeRolledBack,
				Err: &pkgcontext.DependencyError{
					Dependency: "runtime openmp",
					Operation:  "ensure plugins",
					Err:        fmt.Errorf("failed to get plugin from net: %w", missing),
				},
			}

			err := reportDependencyCommandError(ctx, "install", failure)
			var got *download.ReleaseAssetNotFoundError
			if !errors.As(err, &got) || got != missing || ExitCode(err) != 1 {
				t.Errorf("reportDependencyCommandError(missing Linux asset) = %v, want preserved asset failure and exit 1", err)
			}

			for _, want := range []string{"Previous dependency state restored", "Southclaws/pawn-yaml:1.0.0", `No release asset matches "yaml.so"`, "Available: yaml.dll", "Choose a release with the required file"} {
				if !strings.Contains(output.String(), want) {
					t.Errorf("reportDependencyCommandError(missing Linux asset) output = %q, want %q", output.String(), want)
				}
			}

			if strings.Contains(output.String(), "runtime openmp: Dependency command failed") || strings.Contains(output.String(), "pawn-yaml:latest") {
				t.Errorf("reportDependencyCommandError(missing Linux asset) output = %q, want selected dependency and release", output.String())
			}

			if gotDetails := strings.Contains(output.String(), "Details:"); gotDetails != verbose {
				t.Errorf("reportDependencyCommandError(verbose=%t) details = %t, want %t", verbose, gotDetails, verbose)
			}
		})
	}
}

func TestReportReleaseWithoutAssets(t *testing.T) {
	t.Parallel()

	ctx := newTestCLIContext("dev")
	var output bytes.Buffer
	ctx.App.Writer = &output

	missing := &download.ReleaseAssetNotFoundError{
		Meta:    versioning.DependencyMeta{User: "fixture", Repo: "plugin", Branch: "main"},
		Tag:     "v1.0.0",
		Pattern: "plugin.so",
	}

	err := reportDependencyCommandError(ctx, "install", fmt.Errorf("download: %w", missing))
	if !errors.Is(err, missing) || ExitCode(err) != 1 {
		t.Errorf("reportDependencyCommandError(empty release) = %v, want preserved asset failure and exit 1", err)
	}

	for _, want := range []string{"fixture/plugin:v1.0.0", `No release asset matches "plugin.so"`, "This release has no assets"} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("reportDependencyCommandError(empty release) output = %q, want %q", output.String(), want)
		}
	}
}
