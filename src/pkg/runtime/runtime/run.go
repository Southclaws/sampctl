package runtime

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"syscall"
	"time"

	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	"github.com/Southclaws/sampctl/src/pkg/runtime/run"
)

var (
	matchPreamble = regexp.MustCompile(`Loaded [0-9]{1,2} filterscripts\.`)
	matchMainEnd  = regexp.MustCompile(`Number of vehicle models\: [0-9]*`)
	matchTestEnd  = regexp.MustCompile(`\*\*\* Tests: (\d+), Fails: (\d+)`)
)

type testResults struct {
	Tests int
	Fails int
}

// termination is an internal instruction for communicating successful or failed runs.
// It contains an error and a boolean to indicate whether or not to terminate the process.
type termination struct {
	err  error
	exit bool
}

// Run handles the actual running of the server process - it collects log output too
func Run(
	ctx context.Context,
	cfg run.Runtime,
	cacheDir string,
	passArgs,
	recover bool,
	output io.Writer,
	input io.Reader,
) (err error) {
	if cfg.Container != nil {
		return RunContainer(ctx, cfg, cacheDir, passArgs, output, input)
	}

	binary := "./" + getServerBinary(cacheDir, cfg.Version, cfg.Platform)
	fullPath := filepath.Join(cfg.WorkingDir, binary)

	print.Verb("starting", binary, "in", cfg.WorkingDir)

	if cfg.IsOpenMP() {
		print.Verb("detected open.mp runtime")
	} else {
		print.Verb("detected sa-mp runtime")
	}

	linkErr := createSpecialLink(cfg)
	if linkErr != nil {
		print.Verb("failed to create special link: ", linkErr)
	}

	return dorun(ctx, fullPath, cfg.Mode, recover, output, input)
}

// nolint:gocyclo
func dorun(
	ctx context.Context,
	binary string,
	runType run.RunMode,
	recover bool,
	output io.Writer,
	input io.Reader,
) (err error) {
	outputReader, outputWriter := io.Pipe()
	streamChan := make(chan string)    // channel for lines of output text
	errChan := make(chan termination)  // channel for sending runtime errors to watchdog
	sigChan := make(chan os.Signal, 1) // channel for capturing host signals

	defer func() {
		errClose := outputWriter.Close()
		if errClose != nil {
			print.Erro("Server output read error:", errClose)
		}
	}()

	readBinaryOutput(
		runType,
		outputReader,
		errChan,
		streamChan,
	)

	print.Verb("running with mode", runType)

	var cmd *exec.Cmd
	go func() {
		cmd = runBinary(
			ctx,
			binary,
			runType,
			recover,
			outputWriter,
			input,
			errChan,
		)
	}()

	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	var term termination
loop:
	for {
		select {
		case line := <-streamChan:
			fmt.Fprintln(output, line)

		case s := <-sigChan:
			term.err = errors.Errorf("received signal: %v", s)
			break loop

		case term = <-errChan:
			break loop
		}
	}
	print.Verb("finished server execution with:", term)

	err = errors.Wrap(term.err, "received runtime error")

	if term.exit {
		if cmd.Process != nil {
			killErr := cmd.Process.Kill()
			if killErr != nil {
				print.Erro("Failed to kill", killErr)
			}
			print.Verb("sent a SIGINT to child process")
		} else {
			print.Verb("not attempting to kill server: cmd.Process is nil")
		}
	}

	print.Verb("finished run() with", err)

	return err
}

func runBinary(
	ctx context.Context,
	binary string,
	runType run.RunMode,
	recover bool,
	outputWriter io.Writer,
	input io.Reader,
	errChan chan termination,
) (cmd *exec.Cmd) {
	var (
		startTime          time.Time     // time of most recent start/restart
		exponentialBackoff = time.Second // exponential backoff cooldown
	)
	for {
		cmd = exec.CommandContext(ctx, binary) //nolint:gas
		cmd.Dir = filepath.Dir(binary)

		startTime = time.Now()
		errInline := platformRun(cmd, outputWriter, input)
		if errInline != nil {
			errChan <- termination{errors.Wrap(errInline, "failed to start server"), false}
		}

		if cmd.Process != nil {
			print.Verb("child exec thread finished, pid:", cmd.Process.Pid, "error:", errInline)
		} else {
			print.Verb("child exec thread finished, error:", errInline)
		}

		if runType == run.Server && recover {
			runTime := time.Since(startTime)

			if runTime < time.Minute {
				exponentialBackoff *= 2
				print.Verb("doubling backoff time", exponentialBackoff)
			} else {
				exponentialBackoff = time.Second
				print.Verb("initial backoff", exponentialBackoff)
			}

			if exponentialBackoff < time.Second*15 {
				print.Warn("crash loop backoff for", exponentialBackoff, "reason:", errInline)
				time.Sleep(exponentialBackoff)
				continue
			}
			errChan <- termination{errors.Errorf("too many crashloops, last error: %v", errInline), false}
		} else {
			errChan <- termination{}
		}

		break
	}

	return
}

func readBinaryOutput(
	runType run.RunMode,
	outputReader *io.PipeReader,
	errChan chan termination,
	streamChan chan string,
) {
	switch runType {
	case run.MainOnly:
		go func() {
			preamble := true
			preambleSpace := false
			scanner := bufio.NewScanner(outputReader)
			for scanner.Scan() {
				line := scanner.Text()

				if matchPreamble.MatchString(line) {
					preamble = false
					preambleSpace = true
					continue
				}
				if preambleSpace {
					preambleSpace = false
					continue
				}

				if matchMainEnd.MatchString(line) {
					errChan <- termination{nil, true}
					break
				}

				if !preamble {
					streamChan <- line
				}
			}
		}()
	case run.YTesting:
		go func() {
			preamble := true
			preambleSpace := false
			scanner := bufio.NewScanner(outputReader)
			for scanner.Scan() {
				line := scanner.Text()

				if matchPreamble.MatchString(line) {
					preamble = false
					preambleSpace = true
					continue
				}
				if preambleSpace {
					preambleSpace = false
					continue
				}

				if matchTestEnd.MatchString(line) {
					results := testResultsFromLine(line)
					if results.Fails > 0 {
						print.Erro(results.Tests, "tests, with:", results.Fails, "failures.")
						errChan <- termination{errors.New("tests failed"), true}
					} else {
						print.Info(results.Tests, "tests passed!")
						errChan <- termination{nil, true} // end the server process, no error
					}

					break
				}

				if !preamble {
					streamChan <- line
				}
			}
		}()
	default:
		runType = run.Server // set default for later use
		go func() {
			scanner := bufio.NewScanner(outputReader)
			for scanner.Scan() {
				line := scanner.Text()
				streamChan <- line
			}
		}()
	}
}

func createSpecialLink(cfg run.Runtime) error {
	rootLink := cfg.RootLink
	scriptfilesPath := path.Join(cfg.WorkingDir, "scriptfiles")
	specialLink := path.Join(scriptfilesPath, "DANGEROUS_SERVER_ROOT")

	if rootLink {
		print.Verb("root link is enabled, so going ahead with creating the special link to root")
	} else {
		err := os.Remove(specialLink)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		}
		return nil
	}

	// Create a symlink from scriptfiles to the root
	if _, err := os.Stat(scriptfilesPath); err != nil {
		print.Verb("scriptfiles folder doesn't exist and is needed for 'DANGEROUS_SERVER_ROOT' symlink")
		err = os.MkdirAll(scriptfilesPath, 0755)
		if err != nil {
			return err
		}
	}

	sfi, err := os.Lstat(specialLink)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			print.Verb("no 'DANGEROUS_SERVER_ROOT' symlink found, so creating it")
			err = os.Symlink(cfg.WorkingDir, specialLink)
			if err != nil {
				return err
			}
		}
		return err
	}

	if sfi.Mode()&os.ModeSymlink == 0 {
		print.Verb("file 'DANGEROUS_SERVER_ROOT' found, but it's not a symlink. Destroying and recreating it")
		err = os.Remove(specialLink)
		if err != nil {
			return err
		}
		err = os.Symlink(cfg.WorkingDir, specialLink)
		if err != nil {
			return err
		}
	}

	return nil
}

func testResultsFromLine(line string) (results testResults) {
	match := matchTestEnd.FindStringSubmatch(line)
	results.Tests, _ = strconv.Atoi(match[1]) //nolint:errcheck
	results.Fails, _ = strconv.Atoi(match[2]) //nolint:errcheck
	return
}
