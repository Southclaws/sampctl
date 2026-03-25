package commands

import (
	"context"

	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/google/go-github/github"
	"github.com/pkg/errors"
	"golang.org/x/oauth2"
	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/src/config"
)

const commandStateKey = "commandState"

type commandState struct {
	cacheDir string
	version  string
	cfg      *config.Config
	gh       *github.Client
	gitAuth  transport.AuthMethod
}

func newCommandState(version, cacheDir string) *commandState {
	return &commandState{
		cacheDir: cacheDir,
		version:  version,
	}
}

func newGitHubClient(token string) *github.Client {
	if token == "" {
		return github.NewClient(nil)
	}

	client := oauth2.NewClient(
		context.Background(),
		oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token}),
	)
	return github.NewClient(client)
}

func getCommandState(c *cli.Context) (*commandState, error) {
	if c == nil || c.App == nil {
		return nil, errors.New("command context is not available")
	}

	state, ok := c.App.Metadata[commandStateKey].(*commandState)
	if !ok || state == nil {
		return nil, errors.New("command state is not available")
	}

	return state, nil
}
