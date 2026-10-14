package compiler

import (
	"context"
	stderrors "errors"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/src/pkg/build"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
)

// CompileWithCommand takes a prepared command and executes it
func CompileWithCommand(
	cmd *exec.Cmd,
	workingDir,
	errorDir string,
	relative bool,
) (problems build.Problems, result build.Result, err error) {
	return CompileWithCommandContext(context.Background(), cmd, workingDir, errorDir, relative)
}

// CompileWithCommandContext runs a compiler while protecting its managed installation.
func CompileWithCommandContext(
	ctx context.Context,
	cmd *exec.Cmd,
	workingDir,
	errorDir string,
	relative bool,
) (problems build.Problems, result build.Result, err error) {
	if err := ctx.Err(); err != nil {
		return nil, result, &ExecutionError{Kind: ExecutionCancelled, Path: cmd.Path, Err: err}
	}

	if cmd.WaitDelay == 0 {
		cmd.WaitDelay = 2 * time.Second
	}

	unlock, err := lockCompilerExecution(ctx, cmd)
	if err != nil {
		return nil, result, err
	}
	defer func() { err = stderrors.Join(err, unlock()) }()

	if errorDir == "" {
		errorDir = workingDir
	}

	errorDir, err = fs.Abs(errorDir)
	if err != nil {
		return nil, build.Result{}, errors.Wrap(err, "failed to resolve error directory")
	}

	workingDir, err = fs.Abs(workingDir)
	if err != nil {
		return nil, build.Result{}, errors.Wrap(err, "failed to resolve working directory")
	}

	outputReader, outputWriter := io.Pipe()
	cmd.Stdout = outputWriter
	cmd.Stderr = outputWriter

	parser := newCompilerOutputParser(outputReader, workingDir, errorDir, relative)
	go parser.Run()

	print.Verb("executing compiler in", workingDir, "as", cmd.Args)
	cmdError := runCompilerCommand(ctx, cmd)

	closeErr := outputWriter.Close()

	problems, result = parser.Wait()
	if err := stderrors.Join(closeErr, parser.err, outputReader.Close()); err != nil {
		return problems, result, &ExecutionError{Kind: ExecutionOutput, Path: cmd.Path, Output: parser.output.String(), Err: stderrors.Join(err, cmdError)}
	}

	if ctx.Err() != nil {
		return problems, result, &ExecutionError{Kind: ExecutionCancelled, Path: cmd.Path, Output: parser.output.String(), Err: stderrors.Join(ctx.Err(), cmdError)}
	}

	for _, problem := range problems {
		if problem.Code == "101" && problem.Severity == build.ProblemFatal {
			if cmdError == nil {
				cmdError = stderrors.New(problem.Description)
			}
			return problems, result, &ExecutionError{Kind: ExecutionOutput, Path: cmd.Path, Output: parser.output.String(), Err: cmdError}
		}
	}

	if cmdError == nil {
		return problems, result, nil
	}

	if stderrors.Is(cmdError, exec.ErrWaitDelay) {
		return problems, result, &ExecutionError{Kind: ExecutionOutput, Path: cmd.Path, Output: parser.output.String(), Err: cmdError}
	}

	var exitErr *exec.ExitError
	if stderrors.As(cmdError, &exitErr) {
		if exitErr.ExitCode() == 1 && (len(problems.Errors()) != 0 || problems.Fatal()) {
			return problems, result, nil
		}
		return problems, result, &ExecutionError{Kind: ExecutionCrash, Path: cmd.Path, Output: parser.output.String(), Err: cmdError}
	}

	kind := ExecutionStart
	if _, pathErr := os.Stat(cmd.Path); stderrors.Is(cmdError, exec.ErrNotFound) || (stderrors.Is(cmdError, os.ErrNotExist) && stderrors.Is(pathErr, os.ErrNotExist)) {
		kind = ExecutionMissing
	}

	return problems, result, &ExecutionError{Kind: kind, Path: cmd.Path, Output: parser.output.String(), Err: cmdError}
}
