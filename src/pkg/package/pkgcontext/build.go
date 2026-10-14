package pkgcontext

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/src/pkg/build"
	"github.com/Southclaws/sampctl/src/pkg/build/compiler"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
)

type BuildOptions struct {
	Name      string
	Ensure    bool
	DryRun    bool
	Relative  bool
	BuildFile string
	Trigger   chan build.Problems
}

// Build compiles a package, dependencies are ensured and a list of paths are sent to the compiler.
func (pcx *PackageContext) Build(
	ctx context.Context,
	options BuildOptions,
) (
	problems build.Problems,
	result build.Result,
	err error,
) {
	config, err := pcx.buildPrepare(ctx, options.Name, options.Ensure, true)
	if err != nil {
		return
	}

	buildNumber, err := readBuildNumber(options.BuildFile)
	if err != nil {
		return
	}

	command, err := pcx.prepareBuildCommand(ctx, *config)
	if err != nil {
		return
	}
	if options.DryRun {
		printBuildCommand(command)
		return
	}

	return pcx.executeBuild(buildExecutionRequest{
		Context:     ctx,
		Config:      *config,
		Command:     command,
		BuildNumber: buildNumber,
		Options:     options,
	})
}

func readBuildNumber(buildFile string) (uint32, error) {
	if buildFile == "" {
		return 0, nil
	}
	return readInt(buildFile)
}

func (pcx *PackageContext) prepareBuildCommand(ctx context.Context, config build.Config) (*exec.Cmd, error) {
	return compiler.PrepareCommand(ctx, compiler.PrepareCommandRequest{
		GitHub:   pcx.GitHub,
		ExecDir:  pcx.Package.LocalPath,
		CacheDir: pcx.CacheDir,
		Platform: pcx.Platform,
		Config:   config,
	})
}

func printBuildCommand(command *exec.Cmd) {
	var environment []string
	for _, value := range command.Env {
		if strings.HasPrefix(value, "LD_LIBRARY_PATH=") || strings.HasPrefix(value, "DYLD_LIBRARY_PATH=") {
			environment = append(environment, value)
		}
	}
	fmt.Println(strings.Join(append(environment, command.Args...), " "))
}

type buildExecutionRequest struct {
	Context     context.Context
	Config      build.Config
	Command     *exec.Cmd
	BuildNumber uint32
	Options     BuildOptions
}

func (pcx *PackageContext) executeBuild(request buildExecutionRequest) (problems build.Problems, result build.Result, err error) {
	if err = compiler.RunPreBuildCommands(request.Context, request.Config, os.Stdout); err != nil {
		print.Erro("Failed to execute pre-build command:", err)
		return nil, build.Result{}, err
	}

	print.Verb("building", pcx.Package, "with", request.Config.Compiler.Version)
	problems, result, err = compiler.CompileWithCommandContext(request.Context, request.Command, request.Config.WorkingDir, pcx.Package.LocalPath, request.Options.Relative)
	if err != nil {
		return nil, build.Result{}, errors.Wrap(err, "failed to compile package entry")
	}

	atomic.AddUint32(&request.BuildNumber, 1)
	writeBuildNumber(request.Options.BuildFile, request.BuildNumber)

	if err = compiler.RunPostBuildCommands(request.Context, request.Config, os.Stdout); err != nil {
		print.Erro("Failed to execute post-build command:", err)
		return problems, result, err
	}

	return problems, result, nil
}

func writeBuildNumber(buildFile string, buildNumber uint32) {
	if buildFile == "" {
		return
	}
	if err := os.WriteFile(buildFile, []byte(fmt.Sprint(buildNumber)), 0o700); err != nil {
		print.Erro("Failed to write buildfile:", err)
	}
}

func readInt(file string) (n uint32, err error) {
	var contents []byte
	if fs.Exists(file) {
		contents, err = os.ReadFile(file)
		if err != nil {
			err = errors.Wrap(err, "failed to read buildfile")
			return
		}
		var result int
		result, err = strconv.Atoi(string(contents))
		if err != nil {
			err = errors.Wrap(err, "failed to interpret buildfile contents as an integer number")
			return
		}
		if result < 0 {
			err = errors.Wrap(err, "build number is not a positive integer")
			return
		}
		n = uint32(result)
	} else {
		err = os.WriteFile(file, []byte("0"), 0o700)
		n = 0
	}
	return
}
