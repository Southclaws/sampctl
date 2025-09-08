package commands

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"path/filepath"

	"github.com/pkg/errors"
	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/util"
)

func autoComplete(c *cli.Context) (err error) {
	var flavour = "bash"
	if c.String("flavour") == "zsh" {
		flavour = "zsh"
	}

	resp, err := http.Get(fmt.Sprintf(
		"https://raw.githubusercontent.com/urfave/cli/master/autocomplete/%s_autocomplete",
		flavour,
	))
	if err != nil {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return errors.Errorf("failed to get bash completion: %s", resp.Status)
	}

	contents, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return
	}

	cacheDir := util.GetConfigDir()

	completionFile := filepath.Join(cacheDir, "autocomplete")

	err = ioutil.WriteFile(completionFile, contents, 0700)
	if err != nil {
		return
	}

	print.Info("Successfully written", flavour, "completion to", completionFile)
	print.Info("To enable, add the following line to your .bashrc file (or equivalent)")
	print.Info("PROG=sampctl source", completionFile)

	return nil
}
