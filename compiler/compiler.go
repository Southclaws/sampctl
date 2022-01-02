// Package compiler provides an API for acquiring the compiler binaries and compiling Pawn code
package compiler

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"github.com/google/go-github/github"
	"github.com/pkg/errors"
	"rs3.io/go/mserr/ntstatus"

	"github.com/Southclaws/sampctl/build"
	"github.com/Southclaws/sampctl/print"
	"github.com/Southclaws/sampctl/util"
)

//nolint:lll
var (
	// matches warnings or errors
	matchCompilerProblem = regexp.MustCompile(`^(.*?)\(([0-9]*)[- 0-9]*\) \: (fatal error|error|warning) [0-9]*\: (.*)$`)

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
	var (
		input  string
		output string
	)

	input = util.FullPath(config.Input)
	output = util.FullPath(config.Output)
	cacheDir = util.FullPath(cacheDir)

	if !util.Exists(input) {
		err = errors.Errorf("no such file '%s'", input)
		return
	}

	if len(config.Plugins) != 0 {
		print.Warn("The use of `plugins` in the build configuration has been disabled and will be removed in the future")
		print.Warn("Please instead use `prebuild` or `postbuild`")
	}

	outputDir := filepath.Dir(output)
	if !util.Exists(outputDir) {
		err = os.MkdirAll(outputDir, 0700)
		if err != nil {
			err = errors.Wrap(err, "failed to create output directory")
			return
		}
	}

	if config.WorkingDir == "" {
		config.WorkingDir = filepath.Dir(input)
	} else {
		config.WorkingDir = util.FullPath(config.WorkingDir)
	}

	runtimeDir := filepath.Join(cacheDir, "pawn", config.Compiler.Version)
	pkg, err := GetCompilerPackage(ctx, gh, config, runtimeDir, platform, cacheDir)
	if err != nil {
		err = errors.Wrap(err, "failed to get compiler package")
		return
	}

	args := []string{
		input,
		"-D" + config.WorkingDir,
		"-o" + output,
	}
	args = append(args, config.Args...)

	includePaths := make(map[string]struct{})
	includeFiles := make(map[string]string)
	includeErrors := []string{}

	var (
		fullPath string
		contents []os.FileInfo
	)
	for _, inc := range config.Includes {
		if filepath.IsAbs(inc) {
			fullPath = inc
		} else {
			fullPath = filepath.Join(execDir, inc)
		}

		// remove duplicates from the include path list
		if _, found := includePaths[fullPath]; found {
			continue
		}
		includePaths[fullPath] = struct{}{}

		print.Verb("using include path", fullPath)
		args = append(args, "-i"+fullPath)

		contents, err = ioutil.ReadDir(fullPath)
		if err != nil {
			err = errors.Wrapf(err, "failed to list dependency include path: %s", inc)
			return
		}

		for _, dependencyFile := range contents {
			fileName := dependencyFile.Name()
			fileExt := filepath.Ext(fileName)
			if fileExt == ".inc" {
				if location, exists := includeFiles[fileName]; exists {
					if location != fullPath {
						includeErrors = append(includeErrors, fmt.Sprintf(
							"Duplicate '%s' found in both\n'%s'\n'%s'\n",
							fileName, location, fullPath,
						))
					}
				} else {
					includeFiles[fileName] = fullPath
				}
			}
		}
	}

	if len(includeErrors) > 0 {
		print.Erro("Dependency include path errors found:")
		for _, errorString := range includeErrors {
			print.Erro(errorString)
		}

		err = errors.New("could not compile due to conflicting filenames located in different include paths")
		return
	}

	for name, value := range config.Constants {
		if strings.HasPrefix(value, "$") {
			variable := os.Getenv(value[1:])
			if variable == "" {
				print.Warn("Build constant", value, "refers to an unset environment variable")
			}
			args = append(args, fmt.Sprintf("%s=%s", name, variable))
		} else {
			args = append(args, fmt.Sprintf("%s=%s", name, value))
		}
	}

	cmd = exec.CommandContext(ctx, filepath.Join(runtimeDir, pkg.Binary), args...) //nolint:gas
	cmd.Env = []string{
		fmt.Sprintf("LD_LIBRARY_PATH=%s", runtimeDir),
		fmt.Sprintf("DYLD_LIBRARY_PATH=%s", runtimeDir),
	}

	return cmd, nil
}

// CompileWithCommand takes a prepared command and executes it
func CompileWithCommand(
	cmd *exec.Cmd,
	workingDir,
	errorDir string,
	relative bool,
) (problems build.Problems, result build.Result, err error) {
	var (
		outputReader, outputWriter = io.Pipe()
		problemChan                = make(chan build.Problem, 2048)
		resultChan                 = make(chan string, 6)
	)

	if errorDir == "" {
		errorDir = util.FullPath(workingDir)
	}

	cmd.Stdout = outputWriter
	cmd.Stderr = outputWriter
	workingDir = util.FullPath(workingDir)

	go watchCompiler(outputReader, workingDir, errorDir, relative, problemChan, resultChan)

	print.Verb("executing compiler in", workingDir, "as", cmd.Env, cmd.Args)
	cmdError := cmd.Run()

	err = outputWriter.Close()
	if err != nil {
		print.Erro("Compiler output read error:", err)
	}

	if cmdError != nil {
		if cmdError.Error() == "exit status 1" {
			// compilation failed with errors and warnings
			err = nil
		} else {
			err = cmdError
			if runtime.GOOS == "windows" && strings.Contains(cmdError.Error(), "exit status") {
				statusCodeStr := strings.Split(cmdError.Error(), " ")[2]
				statusCodeInt, innerError := strconv.Atoi(statusCodeStr)
				if innerError != nil {
					return
				}

				statusCode := ntstatus.NTStatus(statusCodeInt)
				err = errors.Errorf("exit status %s\n%s", statusCode.String(), statusCode.Error())
			}
			return
		}
	}

	for problem := range problemChan {
		fmt.Println(problem)
		problems = append(problems, problem)
	}

	//nolint:errcheck
	for line := range resultChan {
		if g := matchHeader.FindStringSubmatch(line); len(g) == 2 {
			result.Header, _ = strconv.Atoi(g[1])
		} else if g := matchCode.FindStringSubmatch(line); len(g) == 2 {
			result.Code, _ = strconv.Atoi(g[1])
		} else if g := matchData.FindStringSubmatch(line); len(g) == 2 {
			result.Data, _ = strconv.Atoi(g[1])
		} else if g := matchStack.FindStringSubmatch(line); len(g) == 3 {
			result.StackHeap, _ = strconv.Atoi(g[1])
			result.Estimate, _ = strconv.Atoi(g[2])
		} else if g := matchTotal.FindStringSubmatch(line); len(g) == 2 {
			result.Total, _ = strconv.Atoi(g[1])
		}
	}

	return problems, result, err
}

func watchCompiler(
	outputReader io.Reader,
	workingDir string,
	errorDir string,
	relative bool,
	problemChan chan build.Problem,
	resultChan chan string,
) {
	var err error
	scanner := bufio.NewScanner(outputReader)
	for scanner.Scan() {
		line := scanner.Text()
		groups := matchCompilerProblem.FindStringSubmatch(line)

		if len(groups) == 5 {
			// output is a warning or error

			problem := build.Problem{}

			if filepath.IsAbs(groups[1]) {
				problem.File = groups[1]
			} else {
				problem.File = filepath.Join(workingDir, groups[1])
			}

			if string(filepath.Separator) != `\` {
				problem.File = strings.Replace(problem.File, "\\", "/", -1)
			}
			problem.File = filepath.Clean(problem.File)
			if relative {
				var rel string
				rel, err = filepath.Rel(errorDir, problem.File)
				if err == nil {
					problem.File = rel
				}
			}

			problem.Line, err = strconv.Atoi(groups[2])
			if err != nil {
				return
			}

			switch groups[3] {
			case "warning":
				problem.Severity = build.ProblemWarning
			case "error":
				problem.Severity = build.ProblemError
			case "fatal error":
				problem.Severity = build.ProblemFatal
			}

			problem.Description = groups[4]

			problemChan <- problem
		} else {
			// output is pre-roll or post-roll
			if strings.HasPrefix(line, "Pawn compiler") {
				continue
			} else if strings.HasPrefix(line, "Compilation aborted") {
				continue
			} else if strings.HasSuffix(line, "Error.") {
				continue
			} else if len(strings.TrimSpace(line)) == 0 {
				continue
			} else {
				resultChan <- line
			}
		}
	}

	// close output channels once scanner is closed
	close(problemChan)
	close(resultChan)
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
