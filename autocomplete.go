package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"

	"github.com/Southclaws/sampctl/download"
	"github.com/Southclaws/sampctl/print"
	"github.com/pkg/errors"
	"gopkg.in/urfave/cli.v1"
)

func autoComplete(c *cli.Context) (err error) {
	var flavour = "bash"
	if c.String("flavour") == "zsh" {
		flavour = "zsh"
	}

	resp, err := http.Get(fmt.Sprintf("https://raw.githubusercontent.com/urfave/cli/master/autocomplete/%s_autocomplete", flavour))
	if err != nil {
		return
	}

	if resp.StatusCode != 200 {
		return errors.Errorf("failed to get bash completion: %s", resp.Status)
	}

	contents, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return
	}

	cacheDir, err := download.GetCacheDir()
	if err != nil {
		print.Erro("Failed to retrieve cache directory path (attempted <user folder>/.samp) ", err)
		return
	}

	completionFile := filepath.Join(cacheDir, "autocomplete")

	err = ioutil.WriteFile(completionFile, contents, 0700)
	if err != nil {
		return
	}

	print.Info("Successfully written", flavour, "completion to", completionFile)
	print.Info("To enable, add the following line to your .bashrc file (or equivalent)")
	print.Info("PROG=sampctl source", completionFile)

	return
}

func lastFlagIs(name string) (is bool) {
	numFlags := len(os.Args)
	if numFlags < 2 {
		return false
	}

	return "--"+name == os.Args[numFlags-2]
}
