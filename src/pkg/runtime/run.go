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
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	run "github.com/Southclaws/sampctl/src/pkg/runtime/config"
)

var (
	matchPreamble = regexp.MustCompile(`Loaded [0-9]{1,2} filterscripts\.`)
	matchMainEnd  = regexp.MustCompile(`Number of vehicle models\: [0-9]*`)
	matchTestEnd  = regexp.MustCompile(`\*\*\* Test(?:s|): (\d+),(?: Check(?:s|): \d+,|) Fail(?:s|): (\d+)`)
)

type RunOptions struct {
	CacheDir string
	PassArgs bool
	Recover  bool
	Output   io.Writer
	Input    io.Reader
}

type testResults struct {
	Tests int
	Fails int
}

type termination struct {
	err  error
	exit bool
}

type runtimeExecution struct {
	binary  string
	runType run.RunMode
	recover bool
	output  io.Writer
	input   io.Reader
}

type binaryRunConfig struct {
	binary       string
	runType      run.RunMode
	recover      bool
	outputWriter io.Writer
	input        io.Reader
	termCh       chan<- termination
	onStart      func(*os.Process)
}

type runtimeTerminationRequest struct {
	Context  context.Context
	Output   io.Writer
	StreamCh <-chan string
	TermCh   <-chan termination
	SigCh    <-chan os.Signal
}

type outputReaderRequest struct {
	Context      context.Context
	RunType      run.RunMode
	OutputReader *io.PipeReader
	TermCh       chan<- termination
	StreamCh     chan<- string
}

type runResultRequest struct {
	RunType run.RunMode
	Recover bool
	RunTime time.Duration
	Backoff time.Duration
	RunErr  error
}

type outputModeState struct {
	preamble      bool
	preambleSpace bool
}

type commandTracker struct {
	mu      sync.Mutex
	process *os.Process
}

// Run handles the actual running of the server process - it collects log output too.
func Run(ctx context.Context, cfg run.Runtime, options RunOptions) error {
	options = options.withDefaults()
	if cfg.Container != nil {
		return RunContainer(ctx, cfg, options)
	}

	binary := "./" + getServerBinary(options.CacheDir, cfg.Version, cfg.Platform)
	fullPath := filepath.Join(cfg.WorkingDir, binary)

	print.Verb("starting", binary, "in", cfg.WorkingDir)
	if cfg.IsOpenMP() {
		print.Verb("detected open.mp runtime")
	} else {
		print.Verb("detected sa-mp runtime")
	}

	if err := createSpecialLink(cfg); err != nil {
		print.Verb("failed to create special link:", err)
	}

	return executeRuntime(ctx, runtimeExecution{
		binary:  fullPath,
		runType: cfg.Mode,
		recover: options.Recover,
		output:  options.Output,
		input:   options.Input,
	})
}

func (options RunOptions) withDefaults() RunOptions {
	if options.Output == nil {
		options.Output = io.Discard
	}
	return options
}

func executeRuntime(ctx context.Context, execCfg runtimeExecution) error {
	outputReader, outputWriter := io.Pipe()
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	termCh := make(chan termination, 1)
	streamCh := make(chan string)
	sigCh := make(chan os.Signal, 1)
	tracker := &commandTracker{}

	readerDone := readBinaryOutput(outputReaderRequest{
		Context:      runCtx,
		RunType:      execCfg.runType,
		OutputReader: outputReader,
		TermCh:       termCh,
		StreamCh:     streamCh,
	})
	runnerDone := startBinaryRunner(runCtx, binaryRunConfig{
		binary:       execCfg.binary,
		runType:      execCfg.runType,
		recover:      execCfg.recover,
		outputWriter: outputWriter,
		input:        execCfg.input,
		termCh:       termCh,
		onStart:      tracker.set,
	})

	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	term := waitForRuntimeTermination(runtimeTerminationRequest{
		Context:  runCtx,
		Output:   execCfg.output,
		StreamCh: streamCh,
		TermCh:   termCh,
		SigCh:    sigCh,
	})
	print.Verb("finished server execution with:", term)

	if shouldKillTrackedProcess(term) {
		killTrackedProcess(tracker.current())
	}

	<-runnerDone
	closeOutputPipe(outputWriter)
	if flushErr := flushRuntimeOutput(execCfg.output, streamCh); flushErr != nil && term.err == nil {
		term.err = flushErr
	}
	<-readerDone
	cancel()

	return wrapRuntimeError(term.err)
}

func shouldKillTrackedProcess(term termination) bool {
	if term.exit {
		return true
	}
	if term.err == nil {
		return false
	}
	if errors.Is(term.err, context.Canceled) || errors.Is(term.err, context.DeadlineExceeded) {
		return true
	}
	if strings.Contains(term.err.Error(), "received signal:") {
		return true
	}
	return strings.Contains(term.err.Error(), "failed to write runtime output")
}

func waitForRuntimeTermination(request runtimeTerminationRequest) termination {
	for {
		select {
		case line, ok := <-request.StreamCh:
			if !ok {
				request.StreamCh = nil
				continue
			}
			if _, err := fmt.Fprintln(request.Output, line); err != nil {
				return termination{err: errors.Wrap(err, "failed to write runtime output")}
			}

		case sig := <-request.SigCh:
			return termination{err: errors.Errorf("received signal: %v", sig)}

		case term := <-request.TermCh:
			return term

		case <-request.Context.Done():
			return termination{err: request.Context.Err()}
		}
	}
}

func flushRuntimeOutput(output io.Writer, streamCh <-chan string) error {
	var writeErr error

	for line := range streamCh {
		if writeErr != nil {
			continue
		}
		if _, err := fmt.Fprintln(output, line); err != nil {
			writeErr = errors.Wrap(err, "failed to write runtime output")
		}
	}

	return writeErr
}

func startBinaryRunner(ctx context.Context, cfg binaryRunConfig) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		runBinary(ctx, cfg)
	}()
	return done
}

func readBinaryOutput(request outputReaderRequest) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer close(request.StreamCh)

		state := outputModeState{
			preamble: request.RunType == run.MainOnly || request.RunType == run.YTesting,
		}
		scanner := bufio.NewScanner(request.OutputReader)
		for scanner.Scan() {
			line, emit, term, stop := processOutputLine(request.RunType, &state, scanner.Text())
			if emit && !sendOutputLine(request.Context, request.StreamCh, line) {
				return
			}
			if term != nil {
				sendTermination(request.Context, request.TermCh, *term)
				return
			}
			if stop {
				return
			}
		}

		if err := scanner.Err(); err != nil && request.Context.Err() == nil {
			print.Erro("Server output read error:", err)
		}
	}()
	return done
}

func processOutputLine(
	runType run.RunMode,
	state *outputModeState,
	line string,
) (string, bool, *termination, bool) {
	switch runType {
	case run.MainOnly:
		return processMainOnlyLine(state, line)
	case run.YTesting:
		return processTestingLine(state, line)
	default:
		return line, true, nil, false
	}
}

func processMainOnlyLine(state *outputModeState, line string) (string, bool, *termination, bool) {
	if shouldSkipPreambleLine(state, line) {
		return "", false, nil, false
	}
	if matchMainEnd.MatchString(line) {
		term := termination{exit: true}
		return "", false, &term, true
	}
	return line, true, nil, false
}

func processTestingLine(state *outputModeState, line string) (string, bool, *termination, bool) {
	if shouldSkipPreambleLine(state, line) {
		return "", false, nil, false
	}
	if !matchTestEnd.MatchString(line) {
		return line, true, nil, false
	}

	results := testResultsFromLine(line)
	if results.Fails > 0 {
		print.Erro(results.Tests, "tests, with:", results.Fails, "failures.")
		term := termination{err: errors.New("tests failed"), exit: true}
		return "", false, &term, true
	}

	print.Info(results.Tests, "tests passed!")
	term := termination{exit: true}
	return "", false, &term, true
}

func shouldSkipPreambleLine(state *outputModeState, line string) bool {
	if matchPreamble.MatchString(line) {
		state.preamble = false
		state.preambleSpace = true
		return true
	}
	if state.preambleSpace {
		state.preambleSpace = false
		return true
	}
	return state.preamble
}

func runBinary(ctx context.Context, cfg binaryRunConfig) {
	backoff := time.Second
	for {
		cmd := exec.CommandContext(ctx, cfg.binary) //nolint:gosec
		cmd.Dir = filepath.Dir(cfg.binary)

		startedAt := time.Now()
		runErr := platformRun(cmd, cfg.outputWriter, cfg.input, cfg.onStart)
		logCommandResult(cmd, runErr)

		term, nextBackoff, retry := evaluateRunResult(runResultRequest{
			RunType: cfg.runType,
			Recover: cfg.recover,
			RunTime: time.Since(startedAt),
			Backoff: backoff,
			RunErr:  runErr,
		})
		backoff = nextBackoff
		if retry {
			if !sleepWithContext(ctx, backoff) {
				return
			}
			continue
		}

		sendTermination(ctx, cfg.termCh, term)
		return
	}
}

func evaluateRunResult(request runResultRequest) (termination, time.Duration, bool) {
	if request.RunType != run.Server || !request.Recover {
		if request.RunErr != nil {
			return termination{err: errors.Wrap(request.RunErr, "failed to start server")}, request.Backoff, false
		}
		return termination{}, request.Backoff, false
	}

	nextBackoff := nextCrashLoopBackoff(request.RunTime, request.Backoff)
	if nextBackoff < 15*time.Second {
		print.Warn("crash loop backoff for", nextBackoff, "reason:", request.RunErr)
		return termination{}, nextBackoff, true
	}

	return termination{
		err: errors.Errorf("too many crashloops, last error: %v", request.RunErr),
	}, nextBackoff, false
}

func nextCrashLoopBackoff(runTime, current time.Duration) time.Duration {
	if runTime < time.Minute {
		next := current * 2
		print.Verb("doubling backoff time", next)
		return next
	}

	print.Verb("initial backoff", time.Second)
	return time.Second
}

func logCommandResult(cmd *exec.Cmd, runErr error) {
	if cmd.Process != nil {
		print.Verb("child exec thread finished, pid:", cmd.Process.Pid, "error:", runErr)
		return
	}
	print.Verb("child exec thread finished, error:", runErr)
}

func sleepWithContext(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}

func sendTermination(ctx context.Context, termCh chan<- termination, term termination) bool {
	select {
	case termCh <- term:
		return true
	case <-ctx.Done():
		return false
	}
}

func sendOutputLine(ctx context.Context, streamCh chan<- string, line string) bool {
	select {
	case streamCh <- line:
		return true
	case <-ctx.Done():
		return false
	}
}

func (tracker *commandTracker) set(process *os.Process) {
	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	tracker.process = process
}

func (tracker *commandTracker) current() *os.Process {
	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	return tracker.process
}

func killTrackedProcess(process *os.Process) {
	if process == nil {
		print.Verb("not attempting to kill server: process is nil")
		return
	}
	if err := terminateRuntimeProcess(process); err != nil {
		print.Erro("Failed to kill", err)
		return
	}
	print.Verb("stopped child process")
}

func closeOutputPipe(writer *io.PipeWriter) {
	if err := writer.Close(); err != nil {
		print.Erro("Server output read error:", err)
	}
}

func wrapRuntimeError(err error) error {
	if err == nil {
		return nil
	}
	print.Verb("finished run() with", err)
	return errors.Wrap(err, "received runtime error")
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
		err = os.MkdirAll(scriptfilesPath, 0o755)
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
