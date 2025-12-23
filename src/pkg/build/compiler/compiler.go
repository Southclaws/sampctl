// Package compiler provides an API for acquiring the compiler binaries and compiling Pawn code
package compiler

import (
	"bufio"
	"context"
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

	"github.com/google/go-github/github"
	"github.com/pkg/errors"
	"rs3.io/go/mserr/ntstatus"

	"github.com/Southclaws/sampctl/src/pkg/build/build"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/download"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
)

//nolint:lll
var (
	// matches warnings or errors
	matchCompilerProblem = regexp.MustCompile(`^(.*?)\(([0-9]*)[- 0-9]*\) \: (fatal error|error|user warning|warning)\s?[0-9]*\: (.*)$`)

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

// CompileSource compiles a given input script to the specified output path using compiler version
func CompileSource(
	ctx context.Context,
	gh *github.Client,
	execDir,
	errorDir,
	cacheDir,
	platform string,
	config build.Config,
	relative bool,
) (
	problems build.Problems,
	result build.Result,
	err error,
) {
	cmd, err := PrepareCommand(ctx, gh, execDir, cacheDir, platform, config)
	if err != nil {
		return
	}

	err = RunPreBuildCommands(ctx, config, os.Stdout)
	if err != nil {
		return
	}

	problems, result, err = CompileWithCommand(cmd, config.WorkingDir, errorDir, relative)
	if err != nil {
		return
	}

	err = RunPostBuildCommands(ctx, config, os.Stdout)
	if err != nil {
		return
	}

	return problems, result, nil
}

// PrepareCommand prepares a build command for compiling the given input script
func PrepareCommand(
	ctx context.Context,
	gh *github.Client,
	execDir,
	cacheDir,
	platform string,
	config build.Config,
) (cmd *exec.Cmd, err error) {
	input, output, workingDir, err := prepareIOPaths(config)
	if err != nil {
		return nil, err
	}

	cacheDir = fs.MustAbs(cacheDir)

	if len(config.Plugins) != 0 {
		print.Warn("The use of `plugins` in the build configuration has been disabled and will be removed in the future")
		print.Warn("Please instead use `prebuild` or `postbuild`")
	}

	pkg, runtimeDir, err := resolveCompilerBinary(ctx, gh, cacheDir, platform, config)
	if err != nil {
		return nil, err
	}

	args := baseCompilerArgs(input, workingDir, output)
	args = append(args, compilerOptionArgs(config)...)

	includeArgs, err := buildIncludeArgs(execDir, config.Includes)
	if err != nil {
		return nil, err
	}
	args = append(args, includeArgs...)

	constantArgs := buildConstantArgs(config.Constants)
	args = append(args, constantArgs...)

	cmd = exec.CommandContext(ctx, filepath.Join(runtimeDir, pkg.Binary), args...) //nolint:gas
	cmd.Env = []string{
		fmt.Sprintf("LD_LIBRARY_PATH=%s", runtimeDir),
		fmt.Sprintf("DYLD_LIBRARY_PATH=%s", runtimeDir),
	}

	return cmd, nil
}

func prepareIOPaths(config build.Config) (input, output, workingDir string, err error) {
	input = fs.MustAbs(config.Input)
	output = fs.MustAbs(config.Output)

	if !fs.Exists(input) {
		return "", "", "", errors.Errorf("no such file '%s'", input)
	}

	outputDir := filepath.Dir(output)
	if !fs.Exists(outputDir) {
		if err = fs.EnsureDir(outputDir, fs.PermDirPrivate); err != nil {
			return "", "", "", errors.Wrap(err, "failed to create output directory")
		}
	}

	if config.WorkingDir == "" {
		workingDir = filepath.Dir(input)
	} else {
		workingDir = fs.MustAbs(config.WorkingDir)
	}

	return input, output, workingDir, nil
}

func resolveCompilerBinary(
	ctx context.Context,
	gh *github.Client,
	cacheDir,
	platform string,
	config build.Config,
) (download.Compiler, string, error) {
	resolved := config.Compiler.ResolveCompilerConfig()
	runtimeDir := filepath.Join(cacheDir, "pawn", resolved.Version)

	if config.Compiler.Path == "" {
		pkg, err := GetCompilerPackage(ctx, gh, resolved, runtimeDir, platform, cacheDir)
		if err != nil {
			return download.Compiler{}, "", errors.Wrap(err, "failed to get compiler package")
		}
		return pkg, runtimeDir, nil
	}

	pkg, err := compilerFromCustomPath(config.Compiler.Path)
	if err != nil {
		return download.Compiler{}, "", err
	}
	return pkg, config.Compiler.Path, nil
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
		return download.Compiler{}, errors.Wrap(err, "compiler path is not invalid")
	}
	if !compilerPathStat.Mode().IsRegular() {
		return download.Compiler{}, errors.New("compiler path does not contain a valid pawn compiler executable")
	}

	return download.Compiler{
		Binary: compilerPath,
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
		print.Erro("Dependency include path errors found:")
		for _, errorString := range includeErrors {
			print.Erro(errorString)
		}
		return nil, errors.New("could not compile due to conflicting filenames located in different include paths")
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
	if errorDir == "" {
		errorDir = fs.MustAbs(workingDir)
	}

	outputReader, outputWriter := io.Pipe()
	cmd.Stdout = outputWriter
	cmd.Stderr = outputWriter
	workingDir = fs.MustAbs(workingDir)

	parser := newCompilerOutputParser(outputReader, workingDir, errorDir, relative)
	go parser.Run()

	print.Verb("executing compiler in", workingDir, "as", cmd.Env, cmd.Args)
	cmdError := cmd.Run()

	err = outputWriter.Close()
	if err != nil {
		print.Erro("Compiler output read error:", err)
	}

	problems, result = parser.Wait()

	if cmdError != nil {
		if cmdError.Error() == "exit status 1" {
			// compilation failed with errors and warnings
			err = nil
		} else {
			err = cmdError
			if runtime.GOOS == "windows" && strings.Contains(cmdError.Error(), "exit status") {
				statusCodeStr := strings.Split(cmdError.Error(), " ")[2]
				statusCodeInt, innerError := strconv.ParseInt(statusCodeStr, 0, 64)
				if innerError != nil {
					return
				}

				statusCode := ntstatus.NTStatus(statusCodeInt)
				err = errors.Errorf("exit status %s", statusCode.String())
			}
			return
		}
	}

	return problems, result, err
}

type compilerOutputParser struct {
	reader     io.Reader
	workingDir string
	errorDir   string
	relative   bool
	done       chan struct{}
	problems   build.Problems
	result     build.Result
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
	for scanner.Scan() {
		p.handleLine(scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		print.Erro("Compiler output read error:", err)
	}
	close(p.done)
}

func (p *compilerOutputParser) Wait() (build.Problems, build.Result) {
	<-p.done
	return p.problems, p.result
}

func (p *compilerOutputParser) handleLine(line string) {
	groups := matchCompilerProblem.FindStringSubmatch(line)
	if len(groups) == 5 {
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

	problem.Description = groups[4]
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
		p.result.Header, _ = strconv.Atoi(g[1])
		return
	}
	if g := matchCode.FindStringSubmatch(line); len(g) == 2 {
		p.result.Code, _ = strconv.Atoi(g[1])
		return
	}
	if g := matchData.FindStringSubmatch(line); len(g) == 2 {
		p.result.Data, _ = strconv.Atoi(g[1])
		return
	}
	if g := matchStack.FindStringSubmatch(line); len(g) == 3 {
		p.result.StackHeap, _ = strconv.Atoi(g[1])
		p.result.Estimate, _ = strconv.Atoi(g[2])
		return
	}
	if g := matchTotal.FindStringSubmatch(line); len(g) == 2 {
		p.result.Total, _ = strconv.Atoi(g[1])
		return
	}

	// preserve unknown result lines for potential debugging
	print.Verb("compiler output:", line)
}

// RunPostBuildCommands executes commands after a build is ran for a certain build config
func RunPostBuildCommands(ctx context.Context, cfg build.Config, output io.Writer) (err error) {
	for _, command := range cfg.PostBuildCommands {
		print.Verb("running post-build commands", command)
		ctxInner, cancel := context.WithCancel(ctx)
		defer cancel()

		cmd := exec.CommandContext(ctxInner, command[0], command[1:]...) //nolint:gas
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
		print.Verb("running pre-build commands", command)
		ctxInner, cancel := context.WithCancel(ctx)
		defer cancel()

		cmd := exec.CommandContext(ctxInner, command[0], command[1:]...) //nolint:gas
		cmd.Stdout = output
		cmd.Stderr = output

		err = cmd.Run()
		if err != nil {
			return
		}
	}

	return
}
