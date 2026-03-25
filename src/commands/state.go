package commands

import (
	"context"
	"runtime"
	"time"

	"github.com/Masterminds/semver"
	"github.com/go-git/go-git/v5/plumbing/transport"
	transporthttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	transportssh "github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"github.com/google/go-github/github"
	"github.com/joho/godotenv"
	"github.com/pkg/errors"
	"golang.org/x/oauth2"
	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/src/config"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	"github.com/Southclaws/sampctl/src/pkg/package/pkgcontext"
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

func (s *commandState) configure(c *cli.Context) error {
	if err := godotenv.Load(".env"); err != nil {
		print.Verb(err)
	}

	if c.GlobalBool("bare") {
		return nil
	}

	if c.GlobalBool("verbose") {
		print.SetVerbose()
		print.Verb("Verbose logging active")
	}
	if runtime.GOOS != "windows" {
		print.SetColoured()
	}

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

func (s *commandState) maybeCheckForUpdates(c *cli.Context) {
	if s.cfg == nil || c.GlobalIsSet("generate-bash-completion") || c.GlobalIsSet("bare") {
		return
	}
	if *s.cfg.HideVersionUpdateMessage {
		return
	}
	if time.Now().Minute()%2 != 0 || time.Now().Second()%2 != 0 {
		return
	}

	s.checkForUpdates()
}

func (s *commandState) checkForUpdates() {
	if s.gh == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	release, _, err := s.gh.Repositories.GetLatestRelease(ctx, "Southclaws", "sampctl")
	if err != nil {
		print.Erro("Failed to check for latest sampctl release:", err)
		return
	}

	latest, err := semver.NewVersion(release.GetTagName())
	if err != nil {
		print.Erro("Failed to interpret latest release tag as a semantic version:", err)
		return
	}

	current, err := semver.NewVersion(s.version)
	if err != nil {
		print.Verb("Failed to interpret this version number as a semantic version:", err)
		return
	}

	if latest.GreaterThan(current) {
		printUpgradeInstructions(latest.String(), s.version)
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

func buildGitAuth(cfg *config.Config) transport.AuthMethod {
	httpAuth := newHTTPAuth(cfg)
	sshAuth, err := transportssh.DefaultAuthBuilder("git")
	if err != nil {
		print.Verb("Failed to set up SSH:", err)
	}

	switch {
	case httpAuth != nil && sshAuth != nil:
		return &pkgcontext.GitMultiAuth{HTTP: httpAuth, SSH: sshAuth}
	case httpAuth != nil:
		return httpAuth
	default:
		return sshAuth
	}
}

func newHTTPAuth(cfg *config.Config) transport.AuthMethod {
	switch {
	case cfg.GitUsername != "" && cfg.GitPassword != "":
		return &transporthttp.BasicAuth{
			Username: cfg.GitUsername,
			Password: cfg.GitPassword,
		}
	case cfg.GitHubToken != "":
		return &transporthttp.BasicAuth{
			Username: "x-access-token",
			Password: cfg.GitHubToken,
		}
	default:
		return nil
	}
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

func printUpgradeInstructions(latestVersion, currentVersion string) {
	print.Info("\n-\n")
	print.Info("sampctl version", latestVersion, "available!")
	print.Info("You are currently using", currentVersion)
	print.Info("To upgrade, use the following command:")

	switch runtime.GOOS {
	case "windows":
		print.Info("  scoop update")
		print.Info("  scoop update sampctl")
	case "linux":
		print.Info("  Debian/Ubuntu based systems:")
		print.Info("  curl https://raw.githubusercontent.com/Southclaws/sampctl/master/scripts/install-deb.sh | sh")
		print.Info("  CentOS/Red Hat based systems")
		print.Info("  curl https://raw.githubusercontent.com/Southclaws/sampctl/master/scripts/install-rpm.sh | sh")
	case "darwin":
		print.Info("  brew update")
		print.Info("  brew upgrade sampctl")
	}

	print.Info("If you have any problems upgrading, please open an issue:")
	print.Info("  https://github.com/Southclaws/sampctl/issues/new")
}
