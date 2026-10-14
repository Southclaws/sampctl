package compiler

import (
	"context"
	stderrors "errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
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

type PrepareCommandRequest struct {
	GitHub   *github.Client
	ExecDir  string
	CacheDir string
	Platform string
	Config   build.Config
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
