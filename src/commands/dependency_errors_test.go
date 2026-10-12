package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/urfave/cli.v1"
	"gopkg.in/yaml.v3"

	"github.com/Southclaws/sampctl/src/config"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/sys/gitcheck"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/pkgcontext"
)

func TestReportDependencyCommandError(t *testing.T) {
	t.Parallel()

	for _, verbose := range []bool{false, true} {
		t.Run(fmt.Sprintf("verbose=%t", verbose), func(t *testing.T) {
			ctx := newTestCLIContext("dev")
			set := flag.NewFlagSet("test", flag.ContinueOnError)
			set.Bool("verbose", verbose, "")
			ctx = cli.NewContext(ctx.App, set, nil)
			var output bytes.Buffer
			ctx.App.Writer = &output
			state, err := getCommandState(ctx)
			require.NoError(t, err)
			state.cfg = &config.Config{GitHubToken: "configured-secret"}
			cause := fmt.Errorf("git remote https://person:password@example.com/user/repo?access_token=query-secret: configured-secret: %w", transport.ErrRepositoryNotFound)
			failure := &pkgcontext.DependencyError{Dependency: "user/repo", Operation: "install", Err: cause}
			err = reportDependencyCommandError(ctx, "install", failure)
			require.Error(t, err)
			assert.ErrorIs(t, err, transport.ErrRepositoryNotFound)
			assert.Equal(t, 1, ExitCode(err))
			assert.Empty(t, err.Error(), "main must not print the reported failure again")
			assert.Equal(t, 1, strings.Count(output.String(), "✗ Dependency installation incomplete"))
			assert.Contains(t, output.String(), "user/repo")
			assert.Contains(t, output.String(), "Check the repository name and your Git credentials.")
			assert.Equal(t, verbose, strings.Contains(output.String(), "Details:"))
			for _, secret := range []string{"person", "password", "query-secret", "configured-secret"} {
				assert.NotContains(t, output.String(), secret)
			}
		})
	}
}

func TestReportDependencyFailuresAndRecovery(t *testing.T) {
	t.Parallel()
	ctx := newTestCLIContext("dev")
	var output bytes.Buffer
	ctx.App.Writer = &output
	failure := errors.Join(
		&pkgcontext.DependencyError{Dependency: "user/first", Operation: "install", Err: transport.ErrRepositoryNotFound},
		&pkgcontext.DependencyError{Dependency: "user/second", Operation: "install", Err: os.ErrPermission},
		&pkgcontext.OperationError{Operation: "rollback", Err: os.ErrPermission},
		&pkgcontext.OperationError{Operation: "cleanup", Err: os.ErrPermission},
	)
	err := reportDependencyCommandError(ctx, "ensure", fmt.Errorf("ensure: %w", failure))
	assert.ErrorIs(t, err, os.ErrPermission)
	assert.Contains(t, output.String(), "user/first")
	assert.Contains(t, output.String(), "user/second")
	assert.Contains(t, output.String(), "Previous project state was not fully restored")
	assert.Contains(t, output.String(), "Cannot remove temporary dependency files")
	assert.NotContains(t, output.String(), "was restored")
	assert.NotContains(t, output.String(), "Details:")
}

func TestReportDependencyCommandCancellation(t *testing.T) {
	t.Parallel()
	ctx := newTestCLIContext("dev")
	var output bytes.Buffer
	ctx.App.Writer = &output
	err := reportDependencyCommandError(ctx, "ensure", &pkgcontext.DependencyError{Dependency: "user/repo", Operation: "install", Err: context.Canceled})
	assert.ErrorIs(t, err, context.Canceled)
	assert.Contains(t, output.String(), "cancelled")
	assert.Contains(t, output.String(), "user/repo")
	assert.NotContains(t, output.String(), "Git credentials")
}

func TestReportDependencyCommandOutputFailure(t *testing.T) {
	t.Parallel()
	ctx := newTestCLIContext("dev")
	ctx.App.Writer = failingDiagnosticWriter{}
	cause := &pkgcontext.OperationError{Operation: "lockfile", Err: fmt.Errorf("Authorization: Bearer secret: %w", os.ErrPermission)}
	err := reportDependencyCommandError(ctx, "install", cause)
	assert.ErrorIs(t, err, os.ErrPermission)
	assert.ErrorIs(t, err, io.ErrClosedPipe)
	assert.NotEmpty(t, err.Error())
	assert.NotContains(t, err.Error(), "secret")
}

func TestReportNestedDependencyRecoveryFailure(t *testing.T) {
	t.Parallel()
	ctx := newTestCLIContext("dev")
	var output bytes.Buffer
	ctx.App.Writer = &output
	cause := &pkgcontext.DependencyError{Dependency: "user/repo", Operation: "install", Err: errors.Join(
		transport.ErrRepositoryNotFound,
		&pkgcontext.OperationError{Operation: "cleanup", Err: os.ErrPermission},
	)}
	err := reportDependencyCommandError(ctx, "install", cause)
	assert.ErrorIs(t, err, os.ErrPermission)
	assert.Contains(t, output.String(), "user/repo")
	assert.Contains(t, output.String(), "Cannot remove temporary dependency files")
}

func TestReportCompletedDependencyOperationWithCleanupFailure(t *testing.T) {
	t.Parallel()
	ctx := newTestCLIContext("dev")
	var output bytes.Buffer
	ctx.App.Writer = &output
	err := reportDependencyCommandError(ctx, "install", &pkgcontext.OperationError{Operation: "cleanup", Err: os.ErrPermission})
	assert.Equal(t, 1, ExitCode(err))
	assert.Contains(t, output.String(), "Dependency operation completed; cleanup failed")
	assert.NotContains(t, output.String(), "Dependency installation incomplete")
}

type failingDiagnosticWriter struct{}

func (failingDiagnosticWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestDiagnosticSecretRedaction(t *testing.T) {
	t.Parallel()
	for _, text := range []string{
		"git https://username:secret@example.com/repo.git failed",
		"GET https://example.com/file?access_token=secret&other=value failed",
		"Authorization: Bearer secret\nRepository unavailable",
		"Proxy-Authorization: Basic secret\nRepository unavailable",
		"X-Api-Key: secret\nRepository unavailable",
		"GET https://example.com/file?X-Amz-Signature=secret&other=value failed",
		`{"password": "secret", "token":"secret"}`,
		`{"password": "first secret second", "token":"first secret second"}`,
		`{"password": "first \"secret\" second"}`,
		`password='first secret second'`,
		"X-Api-Key: first secret second\nRepository unavailable",
		"X-Auth-Token: first secret second\nRepository unavailable",
		"credential ghp_123456789012345678901234567890123456",
		"credential github_pat_123456789012345678901234567890123456",
	} {
		t.Run(text, func(t *testing.T) {
			var output bytes.Buffer
			_, err := writeCheckResults(&output, []diagnostic{{Status: checkFailed, Message: "Dependency unavailable", Err: errors.New(text)}}, checkOutputOptions{Verbose: true})
			require.NoError(t, err)
			assert.NotContains(t, output.String(), "secret")
			assert.NotContains(t, output.String(), "username")
			assert.NotContains(t, output.String(), "1234567890")
			assert.Contains(t, output.String(), "[redacted]")
		})
	}
}

func TestDependencyConfigurationDiagnostics(t *testing.T) {
	t.Parallel()
	jsonSyntax := json.Unmarshal([]byte("{broken"), &map[string]string{})
	jsonType := json.Unmarshal([]byte(`{"value": []}`), &map[string]string{})
	yamlType := yaml.Unmarshal([]byte("value: []\n"), &map[string]string{})
	for _, test := range []struct {
		name      string
		failure   error
		message   string
		forbidden string
	}{
		{"lockfile JSON syntax", &pkgcontext.OperationError{Operation: "read lockfile", Err: jsonSyntax}, "pawn.lock is invalid", "write access"},
		{"lockfile JSON type", &pkgcontext.OperationError{Operation: "read lockfile", Err: jsonType}, "pawn.lock is invalid", "write access"},
		{"lockfile unsupported", &pkgcontext.OperationError{Operation: "read lockfile", Err: errors.New("unsupported version")}, "Cannot read pawn.lock", "write access"},
		{"manifest read access", &pkgcontext.OperationError{Operation: "read manifest", Err: os.ErrPermission}, "Cannot read project configuration", "write access"},
		{"manifest load syntax", &pkgcontext.OperationError{Operation: "read manifest", Err: jsonSyntax}, "Project configuration is invalid", "write access"},
		{"manifest JSON syntax", jsonSyntax, "Package configuration is invalid", "write access"},
		{"manifest YAML type", yamlType, "Package configuration is invalid", "write access"},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := newTestCLIContext("dev")
			var output bytes.Buffer
			ctx.App.Writer = &output
			err := reportDependencyCommandError(ctx, "ensure", test.failure)
			assert.ErrorIs(t, err, test.failure)
			assert.Contains(t, output.String(), test.message)
			assert.NotContains(t, output.String(), test.forbidden)
			assert.NotContains(t, output.String(), "Details:")
		})
	}
}

func TestRunPackageEnsurePropagatesLockfileSaveFailure(t *testing.T) {
	t.Parallel()
	target := &fakeEnsureCommandTarget{fakeCommandLockfile: fakeCommandLockfile{hasResolver: true, saveErr: os.ErrPermission}}
	err := runPackageEnsure(context.Background(), target, ensureCommandOptions{useLockfile: true, lockOnly: true})
	assert.ErrorIs(t, err, os.ErrPermission)
	assert.Equal(t, 1, ExitCode(err))
	assert.True(t, target.saved)
	assert.False(t, target.ensureCalled)
}

func TestRunPackageEnsureFailureDoesNotPersistLockfile(t *testing.T) {
	t.Parallel()
	cause := &pkgcontext.DependencyError{Dependency: "user/repo", Operation: "install", Err: transport.ErrRepositoryNotFound}
	target := &fakeEnsureCommandTarget{fakeCommandLockfile: fakeCommandLockfile{hasResolver: true}, ensureErr: cause}
	err := runPackageEnsure(context.Background(), target, ensureCommandOptions{useLockfile: true})
	assert.ErrorIs(t, err, transport.ErrRepositoryNotFound)
	assert.False(t, target.saved)
}

type fakeTransactionalEnsureCommandTarget struct {
	fakeEnsureCommandTarget
	updateProjectCalled bool
	updateProjectErr    error
}

func (f *fakeTransactionalEnsureCommandTarget) UpdateProjectLockfile(context.Context, pkgcontext.DependencyUpdateRequest) error {
	f.updateProjectCalled = true
	return f.updateProjectErr
}

func TestRunPackageEnsureUsesTransactionalLockfileUpdate(t *testing.T) {
	t.Parallel()
	target := &fakeTransactionalEnsureCommandTarget{
		fakeEnsureCommandTarget: fakeEnsureCommandTarget{fakeCommandLockfile: fakeCommandLockfile{hasResolver: true}},
		updateProjectErr:        os.ErrPermission,
	}
	err := runPackageEnsure(context.Background(), target, ensureCommandOptions{useLockfile: true, lockOnly: true})
	assert.ErrorIs(t, err, os.ErrPermission)
	assert.True(t, target.updateProjectCalled)
	assert.False(t, target.updateLockfileCalled)
	assert.False(t, target.saved, "the transaction owns lockfile persistence")
}

func TestCommandExitErrorRedactsUnreportedDetails(t *testing.T) {
	t.Parallel()
	cause := errors.New("selector https://person:secret@example.com/repo?token=query-secret")
	err := &commandExitError{code: 1, err: cause}
	assert.ErrorIs(t, err, cause)
	assert.NotContains(t, err.Error(), "person")
	assert.NotContains(t, err.Error(), "secret")
	assert.Contains(t, err.Error(), "[redacted]")
}

func TestPackageEnsureValidationRemainsReadable(t *testing.T) {
	t.Parallel()
	if !gitcheck.IsInstalled() {
		t.Skip("git is unavailable")
	}
	ctx := newTestCLIContext("dev")
	set := flag.NewFlagSet("test", flag.ContinueOnError)
	set.Bool("force", true, "")
	ctx = cli.NewContext(ctx.App, set, nil)
	err := packageEnsure(ctx)
	require.EqualError(t, err, "cannot use --force without --update")
	assert.Equal(t, 1, ExitCode(err))
	var exitErr *commandExitError
	assert.ErrorAs(t, err, &exitErr)
}

func TestDependencyVerboseDetailsIncludeGitCause(t *testing.T) {
	cause := errors.New("checkout object missing in /cache/objects; https://user:private@example.com/repo?token=private-query")
	err := &pkgcontext.DependencyError{Dependency: "fixture/dep", Operation: "clone", Err: pkgcontext.WrapGitError(cause, versioning.DependencyMeta{User: "fixture", Repo: "dep"})}
	details := checkErrorDetail(err)
	assert.Contains(t, details, "checkout object missing in /cache/objects")
	rendered := redactDiagnosticText(details)
	assert.NotContains(t, rendered, "private")
	assert.Contains(t, rendered, "[redacted]")
}
