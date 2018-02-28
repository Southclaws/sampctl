package runtime

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"syscall"
	"time"

	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/print"
	"github.com/Southclaws/sampctl/types"
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

// Run handles the actual running of the server process - it collects log output too
func Run(ctx context.Context, cfg types.Runtime, cacheDir string, passArgs bool, output io.Writer, input io.Reader) (err error) {
	if cfg.Container != nil {
		return RunContainer(ctx, cfg, cacheDir, passArgs, output, input)
	}

	binary := "./" + getServerBinary(cfg.Platform)
	fullPath := filepath.Join(cfg.WorkingDir, binary)
	print.Verb("starting", binary, "in", cfg.WorkingDir)

	return run(ctx, fullPath, cfg.Mode, output, input)
}

// nolint:gocyclo
func run(ctx context.Context, binary string, runType types.RunMode, output io.Writer, input io.Reader) (err error) {
	// termination is an internal instruction for communicating successful or failed runs.
	// It contains an error and a boolean to indicate whether or not to terminate the process.
	type termination struct {
		err  error
		exit bool
	}

	outputReader, outputWriter := io.Pipe()
	errChan := make(chan termination)  // channel for sending runtime errors to watchdog
	sigChan := make(chan os.Signal, 1) // channel for capturing host signals

	defer func() {
		errClose := outputWriter.Close()
		if errClose != nil {
			print.Erro("Server output read error:", errClose)
		}
	}()

	switch runType {
	case types.MainOnly:
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
					fmt.Fprintln(output, line)
				}
			}
		}()
	case types.YTesting:
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
					fmt.Fprintln(output, line)
				}
			}
		}()
	default:
		runType = types.Server // set default for later use
		go func() {
			scanner := bufio.NewScanner(outputReader)
			for scanner.Scan() {
				line := scanner.Text()
				fmt.Fprintln(output, line)
			}
		}()
	}

	print.Verb("running with mode", runType)

	var cmd *exec.Cmd
	go func() {
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

			if runType == types.Server {
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
	}()

	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	var term termination
	select {
	case s := <-sigChan:
		term.err = errors.Errorf("received signal: %v", s)
	case term = <-errChan:
		break
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

func testResultsFromLine(line string) (results testResults) {
	match := matchTestEnd.FindStringSubmatch(line)
	results.Tests, _ = strconv.Atoi(match[1])
	results.Fails, _ = strconv.Atoi(match[2])
	return
}
