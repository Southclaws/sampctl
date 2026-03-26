package commands

import (
	"runtime"

	"github.com/joho/godotenv"
	"github.com/pkg/errors"
	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/src/config"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
)

func (s *commandState) configure(c *cli.Context) error {
	loadDotEnv()

	if c.GlobalBool("bare") {
		return nil
	}

	configureLogging(c)

	cfg, err := config.LoadOrCreateConfig(s.cacheDir)
	if err != nil {
		return errors.Wrapf(err, "failed to load or create sampctl config in %s", s.cacheDir)
	}

	s.cfg = cfg
	s.gh = newGitHubClient(cfg.GitHubToken)
	s.gitAuth = buildGitAuth(cfg)

	return nil
}

func (s *commandState) saveConfig() error {
	if s.cfg == nil {
		return nil
	}

	if err := config.WriteConfig(s.cacheDir, *s.cfg); err != nil {
		return errors.Wrapf(err, "failed to write updated configuration file to %s", s.cacheDir)
	}

	return nil
}

func loadDotEnv() {
	if err := godotenv.Load(".env"); err != nil {
		print.Verb(err)
	}
}

func configureLogging(c *cli.Context) {
	if c.GlobalBool("verbose") {
		print.SetVerbose()
		print.Verb("Verbose logging active")
	}
	if runtime.GOOS != "windows" {
		print.SetColoured()
	}
}
