// Package compiler provides an API for acquiring the compiler binaries and compiling Pawn code
package compiler

import (
	"bufio"
	"context"
	stderrors "errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/google/go-github/github"
	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/src/pkg/build"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/download"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
)

//nolint:lll
var (
	// matches warnings or errors
	matchCompilerProblem = regexp.MustCompile(`^(.*?)\(([0-9]*)[- 0-9]*\) \: (fatal error|error|user warning|warning)\s?([0-9]*)\: (.*)$`)

	// Header size:             60 bytes
	matchHeader = regexp.MustCompile(`^Header size:\s*([0-9]+) bytes$`)

	// Code size:              276 bytes
	matchCode = regexp.MustCompile(`^Code size:\s*([0-9]+) bytes$`)

	// Data size:                0 bytes
	matchData = regexp.MustCompile(`^Data size:\s*([0-9]+) bytes$`)

	// Stack/heap size:      16384 bytes; estimated max. usage=8 cells (32 bytes)
	matchStack = regexp.MustCompile(`^Stack/heap size:\s*([0-9]*) bytes; estimated max. usage=[0-9]+ cells \(([0-9]+) bytes\)$`)

	// Total requirements:   16720 bytes
	matchTotal = regexp.MustCompile(`^Total requirements:\s*([0-9]+) bytes$`)
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

type PrepareCommandRequest struct {
	GitHub   *github.Client
	ExecDir  string
	CacheDir string
	Platform string
	Config   build.Config
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

// PrepareCommand prepares a build command for compiling the given input script
func PrepareCommand(ctx context.Context, request PrepareCommandRequest) (cmd *exec.Cmd, err error) {
	input, output, workingDir, err := prepareIOPaths(request.Config)
	if err != nil {
		return nil, err
	}

	cacheDir, err := fs.Abs(request.CacheDir)
	if err != nil {
		return nil, errors.Wrap(err, "failed to resolve cache directory")
	}

	if len(request.Config.Plugins) != 0 {
		print.Warn("Build `plugins` are disabled and will be removed. Use `prebuild` or `postbuild`")
	}

	pkg, runtimeDir, err := resolveCompilerBinary(ctx, request, cacheDir)
	if err != nil {
		return nil, err
	}

	args := baseCompilerArgs(input, workingDir, output)
	args = append(args, compilerOptionArgs(request.Config)...)

	includeArgs, err := buildIncludeArgs(request.ExecDir, request.Config.Includes)
	if err != nil {
		return nil, err
	}
	args = append(args, includeArgs...)

	constantArgs := buildConstantArgs(request.Config.Constants)
	args = append(args, constantArgs...)

	cmd = exec.CommandContext(ctx, filepath.Join(runtimeDir, pkg.Binary), args...) //nolint:gosec
	cmd.WaitDelay = 2 * time.Second
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("LD_LIBRARY_PATH=%s", runtimeDir),
		fmt.Sprintf("DYLD_LIBRARY_PATH=%s", runtimeDir),
	)

	return cmd, nil
}

func prepareIOPaths(config build.Config) (input, output, workingDir string, err error) {
	input, err = fs.Abs(config.Input)
	if err != nil {
		return "", "", "", errors.Wrap(err, "failed to resolve input path")
	}

	output, err = fs.Abs(config.Output)
	if err != nil {
		return "", "", "", errors.Wrap(err, "failed to resolve output path")
	}

	if !fs.Exists(input) {
		return "", "", "", errors.Errorf("no such file '%s'", input)
	}

	outputDir := filepath.Dir(output)
	if !fs.Exists(outputDir) {
		if err = fs.EnsureDir(outputDir, fs.PermDirPrivate); err != nil {
			return "", "", "", &ExecutionError{Kind: ExecutionOutput, Path: output, Err: err}
		}
	}

	if info, statErr := os.Stat(outputDir); statErr != nil || !info.IsDir() {
		if statErr == nil {
			statErr = fmt.Errorf("output parent is not a directory")
		}
		return "", "", "", &ExecutionError{Kind: ExecutionOutput, Path: output, Err: statErr}
	}

	if config.WorkingDir == "" {
		workingDir = filepath.Dir(input)
	} else {
		workingDir, err = fs.Abs(config.WorkingDir)
		if err != nil {
			return "", "", "", errors.Wrap(err, "failed to resolve working directory")
		}
	}

	return input, output, workingDir, nil
}

func resolveCompilerBinary(
	ctx context.Context,
	request PrepareCommandRequest,
	cacheDir string,
) (download.Compiler, string, error) {
	resolved := request.Config.Compiler.ResolveCompilerConfig()
	if request.Config.Compiler.Path == "" {
		for _, component := range []string{resolved.User, resolved.Repo, resolved.Version} {
			if !compilerRelativePath(component) || filepath.Base(component) != component {
				return download.Compiler{}, "", &InstallationError{Path: cacheDir, Err: errors.New("invalid compiler cache path")}
			}
		}
	}

	runtimeDir := compilerRuntimeDir(cacheDir, resolved)

	if request.Config.Compiler.Path == "" {
		pkg, err := GetCompilerPackage(ctx, CompilerFetchRequest{
			GitHub: request.GitHub,
			Meta: versioning.DependencyMeta{
				Site: resolved.Site,
				User: resolved.User,
				Repo: resolved.Repo,
				Tag:  resolved.Version,
			},
			Dir:      runtimeDir,
			Platform: request.Platform,
			CacheDir: cacheDir,
		})
		if err != nil {
			return download.Compiler{}, "", errors.Wrap(err, "failed to get compiler package")
		}

		return pkg, runtimeDir, nil
	}

	pkg, err := compilerFromCustomPath(request.Config.Compiler.Path)
	if err != nil {
		return download.Compiler{}, "", err
	}

	return pkg, request.Config.Compiler.Path, nil
}

func compilerRuntimeDir(cacheDir string, resolved build.CompilerConfig) string {
	return filepath.Join(cacheDir, "pawn", resolved.User, resolved.Repo, resolved.Version)
}

func compilerFromCustomPath(pathRoot string) (download.Compiler, error) {
	print.Verb("using custom path for compiler", pathRoot)
	pathStat, err := os.Stat(pathRoot)
	if err != nil {
		return download.Compiler{}, errors.Wrap(err, "compiler path is not valid")
	}

	if !pathStat.IsDir() {
		return download.Compiler{}, errors.New("compiler path is not a valid directory")
	}

	compilerPath := customCompilerBinary(pathRoot)
	compilerPathStat, err := os.Stat(compilerPath)
	if err != nil {
		kind := ExecutionStart
		if stderrors.Is(err, os.ErrNotExist) {
			kind = ExecutionMissing
		}
		return download.Compiler{}, &ExecutionError{Kind: kind, Path: compilerPath, Err: err}
	}

	if !compilerPathStat.Mode().IsRegular() {
		return download.Compiler{}, errors.New("compiler path does not contain a Pawn compiler executable")
	}

	return download.Compiler{
		Binary: filepath.Base(compilerPath),
		Paths:  map[string]string{},
	}, nil
}

func customCompilerBinary(pathRoot string) string {
	if runtime.GOOS == "windows" {
		return path.Join(pathRoot, "pawncc.exe")
	}

	return path.Join(pathRoot, "pawncc")
}

func baseCompilerArgs(input, workingDir, output string) []string {
	return []string{
		input,
		"-D" + workingDir,
		"-o" + output,
	}
}

func compilerOptionArgs(config build.Config) []string {
	if config.Options != nil {
		return append([]string{}, config.Options.ToArgs()...)
	}

	return append([]string{}, config.Args...)
}

func buildIncludeArgs(execDir string, includes []string) ([]string, error) {
	includePaths := make(map[string]struct{})
	includeFiles := make(map[string]string)
	includeErrors := []string{}
	args := make([]string, 0, len(includes))

	for _, inc := range includes {
		fullPath := inc
		if !filepath.IsAbs(inc) {
			fullPath = filepath.Join(execDir, inc)
		}

		if _, found := includePaths[fullPath]; found {
			continue
		}
		includePaths[fullPath] = struct{}{}

		print.Verb("using include path", fullPath)
		args = append(args, "-i"+fullPath)

		contents, err := os.ReadDir(fullPath)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to list dependency include path: %s", inc)
		}

		for _, dependencyFile := range contents {
			if filepath.Ext(dependencyFile.Name()) != ".inc" {
				continue
			}

			if location, exists := includeFiles[dependencyFile.Name()]; exists {
				if location != fullPath {
					includeErrors = append(includeErrors, fmt.Sprintf(
						"Duplicate '%s' found in both\n'%s'\n'%s'\n",
						dependencyFile.Name(), location, fullPath,
					))
				}
			} else {
				includeFiles[dependencyFile.Name()] = fullPath
			}
		}
	}

	if len(includeErrors) > 0 {
		print.Erro("Conflicting include files:")
		for _, errorString := range includeErrors {
			print.Erro(errorString)
		}
		return nil, errors.New("conflicting filenames in include paths. Keep one copy of each include file")
	}

	return args, nil
}

func buildConstantArgs(constants map[string]string) []string {
	args := make([]string, 0, len(constants))
	for name, value := range constants {
		finalValue := resolveConstantValue(value)
		if isNumeric(finalValue) || finalValue == "" {
			args = append(args, fmt.Sprintf("%s=%s", name, finalValue))
			continue
		}

		escapedValue := strings.ReplaceAll(finalValue, `"`, `\\"`)
		args = append(args, fmt.Sprintf("%s=\"%s\"", name, escapedValue))
	}

	return args
}

func resolveConstantValue(value string) string {
	if !strings.HasPrefix(value, "$") {
		return value
	}

	translated := os.Getenv(value[1:])
	if translated == "" {
		print.Warn("Build constant", value, "refers to an unset environment variable")
	}

	return translated
}

func isNumeric(value string) bool {
	if _, err := strconv.Atoi(value); err == nil {
		return true
	}

	if _, err := strconv.ParseFloat(value, 64); err == nil {
		return true
	}

	return false
}

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

type compilerOutputParser struct {
	reader     io.Reader
	workingDir string
	errorDir   string
	relative   bool
	done       chan struct{}
	problems   build.Problems
	result     build.Result
	err        error
	output     strings.Builder
}

func newCompilerOutputParser(reader io.Reader, workingDir, errorDir string, relative bool) *compilerOutputParser {
	return &compilerOutputParser{
		reader:     reader,
		workingDir: workingDir,
		errorDir:   errorDir,
		relative:   relative,
		done:       make(chan struct{}),
	}
}

func (p *compilerOutputParser) Run() {
	scanner := bufio.NewScanner(p.reader)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	for scanner.Scan() {
		if p.output.Len() < 64*1024 {
			p.output.WriteString(scanner.Text())
			p.output.WriteByte('\n')
		}
		p.handleLine(scanner.Text())
	}

	if p.err = scanner.Err(); p.err != nil {
		// Drain the pipe so a compiler with invalid output can still exit.
		_, drainErr := io.Copy(io.Discard, p.reader)
		p.err = stderrors.Join(p.err, drainErr)
	}

	close(p.done)
}

func (p *compilerOutputParser) Wait() (build.Problems, build.Result) {
	<-p.done
	return p.problems, p.result
}

func (p *compilerOutputParser) handleLine(line string) {
	groups := matchCompilerProblem.FindStringSubmatch(line)
	if len(groups) == 6 {
		p.handleProblem(groups)
		return
	}

	p.handleResultLine(line)
}

func (p *compilerOutputParser) handleProblem(groups []string) {
	problem := build.Problem{}
	if filepath.IsAbs(groups[1]) {
		problem.File = groups[1]
	} else {
		problem.File = filepath.Join(p.workingDir, groups[1])
	}

	if string(filepath.Separator) != `\\` {
		problem.File = strings.ReplaceAll(problem.File, "\\", "/")
	}
	problem.File = filepath.Clean(problem.File)
	if p.relative {
		if rel, err := filepath.Rel(p.errorDir, problem.File); err == nil {
			problem.File = rel
		}
	}

	lineNumber, err := strconv.Atoi(groups[2])
	if err != nil {
		return
	}
	problem.Line = lineNumber

	switch groups[3] {
	case "user warning":
		fallthrough
	case "warning":
		problem.Severity = build.ProblemWarning
	case "error":
		problem.Severity = build.ProblemError
	case "fatal error":
		problem.Severity = build.ProblemFatal
	}

	problem.Code = groups[4]
	problem.Description = groups[5]
	fmt.Println(problem.String())
	p.problems = append(p.problems, problem)
}

func (p *compilerOutputParser) handleResultLine(line string) {
	switch {
	case strings.HasPrefix(line, "Pawn compiler"):
		return
	case strings.HasPrefix(line, "Compilation aborted"):
		return
	case strings.HasSuffix(line, "Error."):
		return
	case len(strings.TrimSpace(line)) == 0:
		return
	}

	if g := matchHeader.FindStringSubmatch(line); len(g) == 2 {
		if value, err := strconv.Atoi(g[1]); err == nil {
			p.result.Header = value
		}
		return
	}

	if g := matchCode.FindStringSubmatch(line); len(g) == 2 {
		if value, err := strconv.Atoi(g[1]); err == nil {
			p.result.Code = value
		}
		return
	}

	if g := matchData.FindStringSubmatch(line); len(g) == 2 {
		if value, err := strconv.Atoi(g[1]); err == nil {
			p.result.Data = value
		}
		return
	}

	if g := matchStack.FindStringSubmatch(line); len(g) == 3 {
		stackHeap, err := strconv.Atoi(g[1])
		if err != nil {
			return
		}

		estimate, err := strconv.Atoi(g[2])
		if err != nil {
			return
		}

		p.result.StackHeap = stackHeap
		p.result.Estimate = estimate
		return
	}

	if g := matchTotal.FindStringSubmatch(line); len(g) == 2 {
		if value, err := strconv.Atoi(g[1]); err == nil {
			p.result.Total = value
		}
		return
	}

	// preserve unknown result lines for potential debugging
	print.Verb("compiler output:", line)
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
