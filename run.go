package main

import (
	"bufio"
	"fmt"
	"log"
	"os/exec"
	"time"

	"github.com/pkg/errors"
)

// Run handles the actual running of the server process - it collects log output too
func Run(endpoint, version, dir string) (err error) {
	empty, errs := validate(dir, version)
	if errs != nil {
		return errors.Errorf("directory in invalid state: %v", errs)
	}
	if empty {
		err := GetPackage(endpoint, version, dir)
		if err != nil {
			return errors.Wrap(err, "failed to get server package")
		}
	}

	server, err := NewConfigFromEnvironment(dir)
	if err != nil {
		return errors.Wrap(err, "failed to generate config from environment")
	}

	errs = server.ValidateWorkspace(dir)
	if errs != nil {
		return errors.Errorf("%v", errs)
	}

	err = server.GenerateServerCfg(dir)
	if err != nil {
		return errors.Wrap(err, "failed to generate server.cfg")
	}

	binary := "./" + getServerBinary()
	fmt.Printf("Starting server process '%s'...\n", binary)

	return watchdog(binary)
}

func watchdog(binary string) (err error) {
	var (
		startTime          time.Time     // time of most recent start/restart
		exponentialBackoff = time.Second // exponential backoff cooldown
	)

	for {
		cmd := exec.Command(binary)
		pipe, err := cmd.StdoutPipe()
		if err != nil {
			return err
		}
		err = cmd.Start()
		if err != nil {
			return err
		}

		startTime = time.Now()
		go func() {
			br := bufio.NewReader(pipe)
			var (
				raw      []byte
				isPrefix bool
				inMulti  bool
				line     string
			)
			for {
				raw, isPrefix, err = br.ReadLine()
				if err != nil {
					break
				}

				if isPrefix {
					if !inMulti {
						inMulti = true
						line = string(raw)
						continue
					} else {
						line += string(raw)
					}
				} else if inMulti {
					inMulti = false
				} else {
					line = string(raw)
				}

				log.Println(line)
			}
		}()

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
