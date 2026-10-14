// Package compiler provides an API for acquiring the compiler binaries and compiling Pawn code
package compiler

import (
	"context"
	"io"
	"os"
	"os/exec"

	"github.com/google/go-github/github"
	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/src/pkg/build"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
)

type CompileRequest struct {
	GitHub   *github.Client
	ExecDir  string
	ErrorDir string
	CacheDir string
	Platform string
	Config   build.Config
	Relative bool
}

// CompileSource compiles a given input script to the specified output path using compiler version
func CompileSource(
	ctx context.Context,
	request CompileRequest,
) (
	problems build.Problems,
	result build.Result,
	err error,
) {
	cmd, err := PrepareCommand(ctx, PrepareCommandRequest{
		GitHub:   request.GitHub,
		ExecDir:  request.ExecDir,
		CacheDir: request.CacheDir,
		Platform: request.Platform,
		Config:   request.Config,
	})
	if err != nil {
		return
	}

	err = RunPreBuildCommands(ctx, request.Config, os.Stdout)
	if err != nil {
		return
	}

	problems, result, err = CompileWithCommandContext(
		ctx,
		cmd,
		request.Config.WorkingDir,
		request.ErrorDir,
		request.Relative,
	)
	if err != nil {
		return
	}

	err = RunPostBuildCommands(ctx, request.Config, os.Stdout)
	if err != nil {
		return
	}

	return problems, result, nil
}

// RunPostBuildCommands executes commands after a build is ran for a certain build config
func RunPostBuildCommands(ctx context.Context, cfg build.Config, output io.Writer) (err error) {
	for _, command := range cfg.PostBuildCommands {
		if len(command) == 0 {
			return errors.New("post-build command is empty")
		}

		print.Verb("running post-build commands", command)
		cmd := exec.CommandContext(ctx, command[0], command[1:]...) //nolint:gosec
		cmd.Stdout = output
		cmd.Stderr = output

		err = cmd.Run()
		if err != nil {
			return
		}
	}

	return
}

// RunPreBuildCommands executes commands before a build is ran for a certain build config
func RunPreBuildCommands(ctx context.Context, cfg build.Config, output io.Writer) (err error) {
	for _, command := range cfg.PreBuildCommands {
		if len(command) == 0 {
			return errors.New("pre-build command is empty")
		}

		print.Verb("running pre-build commands", command)
		cmd := exec.CommandContext(ctx, command[0], command[1:]...) //nolint:gosec
		cmd.Stdout = output
		cmd.Stderr = output

		err = cmd.Run()
		if err != nil {
			return
		}
	}

	return
}
