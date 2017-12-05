package runtime

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/pkg/errors"
)

// Run handles the actual running of the server process - it collects log output too
func Run(endpoint, version, dir string) (err error) {
	server, err := NewConfigFromEnvironment(dir)
	if err != nil {
		return errors.Wrap(err, "failed to generate config from environment")
	}

	errs := server.ValidateWorkspace(dir)
	if errs != nil {
		return errors.Errorf("%v", errs)
	}

	err = server.GenerateServerCfg(dir)
	if err != nil {
		return errors.Wrap(err, "failed to generate server.cfg")
	}

	binary := "./" + getServerBinary()
	fullPath := filepath.Join(dir, binary)
	fmt.Printf("start %s in %s\n", binary, dir)

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

		fmt.Printf("crash loop exponential backoff: %s: %v\n", exponentialBackoff, err)
		time.Sleep(exponentialBackoff)
	}
}
