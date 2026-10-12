package commands

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/src/config"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/sys/gitcheck"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
)

type checkStatus string

const (
	checkPassed   checkStatus = "PASS"
	checkWarning  checkStatus = "WARN"
	checkFailed   checkStatus = "FAIL"
	checkSkipped  checkStatus = "SKIP"
	checkRepaired checkStatus = "REPAIRED"
)

type diagnostic struct {
	Name            string
	Status          checkStatus
	Message         string
	Action          string
	Path            string
	Err             error
	CanFix          bool
	RepairAttempted bool
	RepairVerified  bool
}

type checkOptions struct {
	Dir      string
	CacheDir string
	Platform string
	Fix      bool
	Verbose  bool
}

func newCheckCommand(global []cli.Flag) cli.Command {
	return cli.Command{
		Name:        "check",
		Usage:       "sampctl check [--fix]",
		Description: "Diagnoses local configuration, dependencies, lockfiles, compilers, and managed caches. --fix repairs only supported managed resources and verifies the result.",
		Action:      projectCheck,
		Flags: withGlobalFlags(global, []cli.Flag{
			cli.BoolFlag{Name: "fix", Usage: "attempt safe repairs of managed resources"},
			cli.StringFlag{Name: "dir", Value: ".", Usage: "project directory (searches parent directories)"},
		}),
		OnUsageError: func(_ *cli.Context, err error, _ bool) error { return err },
	}
}

func projectCheck(c *cli.Context) error {
	if c.NArg() != 0 {
		return &commandExitError{code: 2, err: errors.New("check does not accept positional arguments; use --dir to select a project")}
	}
	state, err := getCommandState(c)
	if err != nil {
		return &commandExitError{code: 2, err: err}
	}
	dir, err := filepath.Abs(c.String("dir"))
	if err != nil {
		return &commandExitError{code: 2, err: fmt.Errorf("resolve project directory: %w", err)}
	}
	info, err := os.Stat(dir)
	if err != nil {
		return &commandExitError{code: 2, err: fmt.Errorf("access project directory: %w", err)}
	}
	if !info.IsDir() {
		return &commandExitError{code: 2, err: fmt.Errorf("project directory %q is not a directory", dir)}
	}
	ctx, cancel := newCommandTimeoutContext(2 * time.Minute)
	defer cancel()
	opts := checkOptions{Dir: dir, CacheDir: state.cacheDir, Platform: checkPlatform(c), Fix: c.Bool("fix"), Verbose: c.Bool("verbose") || c.GlobalBool("verbose")}
	if opts.Platform != "linux" && opts.Platform != "windows" && opts.Platform != "darwin" {
		return &commandExitError{code: 2, err: fmt.Errorf("unsupported check platform %q; use linux, windows, or darwin", opts.Platform)}
	}
	results, checkErr := runChecks(ctx, opts)
	if checkErr != nil {
		results = append(results, diagnostic{Name: "check", Status: checkFailed, Message: "Diagnostics did not complete", Err: checkErr})
	}
	code, err := writeCheckResults(c.App.Writer, results, checkOutputOptions{Verbose: opts.Verbose, Bare: c.Bool("bare") || c.GlobalBool("bare")})
	if err != nil {
		return &commandExitError{code: 2, err: errors.Join(checkErr, err)}
	}
	if checkErr != nil {
		return &commandExitError{code: 2, err: checkErr, reported: true}
	}
	if code != 0 {
		return &commandExitError{code: code}
	}
	return nil
}

func runChecks(ctx context.Context, opts checkOptions) ([]diagnostic, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	results := []diagnostic{checkGlobalConfig(opts.CacheDir)}
	pkg, project := checkProject(opts.Dir)
	var overrides map[string]string
	var overrideErr error
	if project.Status == checkPassed {
		overrides, overrideErr = versioning.LoadLocalDependencyOverrides(opts.CacheDir)
	}
	results = append(results, project, checkGit(overrideErr == nil && checkDependenciesNeedGit(pkg, overrides)))
	cacheResults, err := checkManagedCache(ctx, opts)
	results = append(results, cacheResults...)
	if err != nil {
		return results, err
	}
	if project.Status != checkPassed {
		for _, name := range []string{"dependencies", "lockfile", "compiler"} {
			results = append(results, diagnostic{Name: name, Status: checkSkipped, Message: name + " check requires a valid project configuration"})
		}
		return results, ctx.Err()
	}
	results = append(results, checkProjectPaths(pkg)...)
	if overrideErr != nil {
		results = append(results, diagnostic{Name: "dependency overrides", Status: checkFailed, Message: "Dependency overrides could not be read", Action: "Correct the dependency override configuration before checking dependencies.", Err: overrideErr})
		overrides = nil
	}
	lf, lockResults := checkLockfile(pkg, overrides)
	results = append(results, lockResults...)
	if overrideErr == nil {
		dependencies, depErr := checkDependencies(ctx, pkg, lf, overrides)
		results = append(results, dependencies...)
		if depErr != nil {
			return results, depErr
		}
	} else {
		results = append(results, diagnostic{Name: "dependencies", Status: checkSkipped, Message: "Dependencies check requires valid dependency overrides"})
	}
	opts.Dir = pkg.LocalPath
	compilers, err := checkCompilers(ctx, pkg, opts)
	if err == nil {
		err = ctx.Err()
	}
	return append(results, compilers...), err
}

func checkPlatform(c *cli.Context) string {
	if selected := c.String("platform"); selected != "" {
		return selected
	}
	if selected := c.GlobalString("platform"); selected != "" {
		return selected
	}
	return runtime.GOOS
}

func checkGlobalConfig(cacheDir string) diagnostic {
	d := diagnostic{Name: "configuration", Status: checkPassed, Message: "Global configuration is valid", Path: cacheDir}
	if _, err := config.LoadConfig(cacheDir); err != nil {
		d.Status = checkFailed
		d.Message = "Global configuration could not be loaded"
		d.Action = "Check config.json or config.yaml in the sampctl configuration directory and SAMPCTL environment variables."
		d.Err = err
	}
	return d
}

func checkGit(required bool) diagnostic {
	d := diagnostic{Name: "git", Status: checkPassed, Message: "Git is available"}
	if gitcheck.IsInstalled() {
		return d
	}
	d.Status = checkWarning
	d.Message = "Git is unavailable; it is needed for dependency operations"
	d.Action = "Install Git and make it available on PATH."
	if required {
		d.Status = checkFailed
	}
	return d
}

func checkDependenciesNeedGit(pkg pawnpackage.Package, overrides map[string]string) bool {
	for _, declaration := range pkg.GetAllDependencies() {
		meta, err := declaration.ExplodeWithOverrides(overrides)
		if err == nil && !meta.IsLocalScheme() {
			return true
		}
	}
	return false
}

func repairDiagnostic(ctx context.Context, d diagnostic, repair func(context.Context) error, verify func() diagnostic) (diagnostic, error) {
	if err := ctx.Err(); err != nil {
		return d, err
	}
	d.RepairAttempted = true
	repairErr := repair(ctx)
	if err := ctx.Err(); err != nil {
		d.Err = errors.Join(d.Err, repairErr)
		if !errors.Is(d.Err, err) {
			d.Err = errors.Join(d.Err, err)
		}
		return d, err
	}
	verified := verify()
	d.RepairVerified = verified.Status == checkPassed
	if repairErr == nil && verified.Status == checkPassed {
		verified.Status = checkRepaired
		verified.CanFix = d.CanFix
		verified.RepairAttempted = true
		verified.RepairVerified = true
		return verified, nil
	}
	d.Status = checkFailed
	d.Err = errors.Join(d.Err, repairErr, verified.Err)
	if repairErr != nil {
		d.Message += "; repair failed"
	} else {
		d.Message += "; repair did not resolve the issue"
	}
	return d, nil
}

func boolCount(value bool) int {
	if value {
		return 1
	}
	return 0
}
