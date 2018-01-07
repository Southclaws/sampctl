package runtime

import (
	"os"
	"os/exec"
	"path/filepath"
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

	return watchdog(fullPath)
}

func watchdog(binary string) (err error) {
	var (
		startTime          time.Time     // time of most recent start/restart
		exponentialBackoff = time.Second // exponential backoff cooldown
	)

	for {
		cmd := exec.Command(binary)
		cmd.Dir = filepath.Dir(binary)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err = cmd.Start()
		if err != nil {
			return err
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
			return errors.Errorf("too many crashloops, last error: %v", err)
		}

		print.Warn("crash loop backoff for", exponentialBackoff, "reason:", err)
		time.Sleep(exponentialBackoff)
	}
}
