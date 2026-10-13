package commands

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
)

func autoComplete(c *cli.Context) (err error) {
	flavour := "bash"
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
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			print.Warn("failed to close autocomplete response body:", closeErr)
		}
	}()

	if resp.StatusCode != 200 {
		return errors.Errorf("failed to download %s completion: %s", flavour, resp.Status)
	}

	contents, err := io.ReadAll(resp.Body)
	if err != nil {
		return
	}

	env, err := getCommandEnv(c)
	if err != nil {
		return err
	}

	completionFile := filepath.Join(env.CacheDir, "autocomplete")

	err = os.WriteFile(completionFile, contents, fs.PermFileExec)
	if err != nil {
		return
	}

	print.Info("Saved", flavour, "completion to", completionFile)
	print.Info("Add this line to your shell configuration (for example, .bashrc):")
	print.Info("PROG=sampctl source", completionFile)

	return nil
}
