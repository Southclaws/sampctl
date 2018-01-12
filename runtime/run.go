package runtime

import (
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/print"
	"github.com/Southclaws/sampctl/types"
)

// Run handles the actual running of the server process - it collects log output too
func Run(cfg types.Runtime, cacheDir string) (err error) {
	if cfg.Container != nil {
		return RunContainer(cfg, cacheDir)
	}

	binary := "./" + getServerBinary(cfg.Platform)
	fullPath := filepath.Join(cfg.WorkingDir, binary)
	print.Verb("starting", binary, "in", cfg.WorkingDir)

	switch cfg.RunType {
	case types.Server:
		err = serverMode(fullPath)
	case types.MainOnly:
		err = mainMode(fullPath)
	}

	return
}

func serverMode(binary string) (err error) {
	var (
		startTime          time.Time     // time of most recent start/restart
		exponentialBackoff = time.Second // exponential backoff cooldown
	)

	cmd := exec.Command(binary)
	cmd.Dir = filepath.Dir(binary)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	errChan := make(chan error)

	go func() {
		for {
			err = cmd.Start()
			if err != nil {
				errChan <- err
				break
			}

			startTime = time.Now()

			// todo: capture output for further processing
			// for scanner.Scan() {
			// 	println(scanner.Text())
			// }

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

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	select {
	case s := <-sig:
		err = errors.Errorf("received signal: %v", s)
	case err = <-errChan:
		break
	}

	return
}

func mainMode(binary string) (err error) {
	return
}
