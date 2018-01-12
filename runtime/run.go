package runtime

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"syscall"
	"time"

	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/print"
	"github.com/Southclaws/sampctl/types"
)

var (
	matchPreamble = regexp.MustCompile(`Loaded [0-9]{1,2} filterscripts\.`)
	matchMainEnd  = regexp.MustCompile(`Number of vehicle models\: [0-9]*`)
)

// Run handles the actual running of the server process - it collects log output too
func Run(cfg types.Runtime, cacheDir string) (err error) {
	if cfg.Container != nil {
		return RunContainer(cfg, cacheDir)
	}

	binary := "./" + getServerBinary(cfg.Platform)
	fullPath := filepath.Join(cfg.WorkingDir, binary)
	print.Verb("starting", binary, "in", cfg.WorkingDir)

	return run(fullPath, cfg.RunType)
}

func run(binary string, runType types.RunType) (err error) {
	outputReader, outputWriter := io.Pipe()
	cmd := exec.Command(binary)
	cmd.Dir = filepath.Dir(binary)
	cmd.Stdout = outputWriter
	cmd.Stderr = outputWriter
	errChan := make(chan error)
	sigChan := make(chan os.Signal, 1)

	defer func() {
		err = outputWriter.Close()
		if err != nil {
			print.Erro("Compiler output read error:", err)
		}
	}()

	if runType == types.Server {
		go func() {
			scanner := bufio.NewScanner(outputReader)
			for scanner.Scan() {
				line := scanner.Text()
				fmt.Println(line)
			}
		}()
	} else if runType == types.MainOnly {
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
					sigChan <- syscall.SIGTERM // end the server process
					break
				}

				if !preamble {
					fmt.Println(line)
				}
			}
		}()
	}

	go func() {
		var (
			startTime          time.Time     // time of most recent start/restart
			exponentialBackoff = time.Second // exponential backoff cooldown
		)
		for {
			err = cmd.Start()
			if err != nil {
				errChan <- err
				break
			}
			startTime = time.Now()
			err = cmd.Wait()

			runTime := time.Since(startTime)
			if runTime < time.Minute {
				exponentialBackoff *= 2
			} else {
				exponentialBackoff = time.Second
			}

			if exponentialBackoff > time.Second*15 {
				errChan <- errors.Errorf("too many crashloops, last error: %v", err)
				break
			}

			print.Warn("crash loop backoff for", exponentialBackoff, "reason:", err)
			time.Sleep(exponentialBackoff)
		}
	}()

	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	select {
	case s := <-sigChan:
		err = cmd.Process.Kill()
		if err != nil {
			err = errors.Wrap(err, "failed to kill process after signal")
		} else {
			err = errors.Errorf("received signal: %v", s)
		}
	case err = <-errChan:
		break
	}

	return
}
