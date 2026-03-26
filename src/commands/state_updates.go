package commands

import (
	"runtime"
	"time"

	"github.com/Masterminds/semver"
	"github.com/pkg/errors"
	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/src/config"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
)

func (s *commandState) maybeCheckForUpdates(c *cli.Context) {
	if !shouldCheckForUpdates(s.cfg, c.GlobalIsSet("generate-bash-completion"), c.GlobalIsSet("bare"), time.Now()) {
		return
	}

	s.checkForUpdates()
}

func (s *commandState) checkForUpdates() {
	if s.gh == nil {
		return
	}

	ctx, cancel := newCommandTimeoutContext(10 * time.Second)
	defer cancel()

	release, _, err := s.gh.Repositories.GetLatestRelease(ctx, "Southclaws", "sampctl")
	if err != nil {
		print.Erro("Failed to check for latest sampctl release:", err)
		return
	}

	upgrade, err := needsUpgrade(s.version, release.GetTagName())
	if err != nil {
		reportUpgradeCheckError(err)
		return
	}

	if upgrade {
		printUpgradeInstructions(release.GetTagName(), s.version)
	}
}

func shouldCheckForUpdates(cfg *config.Config, generateCompletion, bare bool, now time.Time) bool {
	if cfg == nil || generateCompletion || bare {
		return false
	}
	if cfg.HideVersionUpdateMessage != nil && *cfg.HideVersionUpdateMessage {
		return false
	}
	if now.Minute()%2 != 0 || now.Second()%2 != 0 {
		return false
	}

	return true
}

func needsUpgrade(currentVersion, latestVersion string) (bool, error) {
	latest, err := semver.NewVersion(latestVersion)
	if err != nil {
		return false, errors.Wrap(err, "failed to interpret latest release tag as a semantic version")
	}

	current, err := semver.NewVersion(currentVersion)
	if err != nil {
		return false, errors.Wrap(err, "failed to interpret this version number as a semantic version")
	}

	return latest.GreaterThan(current), nil
}

func reportUpgradeCheckError(err error) {
	if err == nil {
		return
	}

	message := err.Error()
	switch {
	case message == "failed to interpret latest release tag as a semantic version":
		print.Erro("Failed to interpret latest release tag as a semantic version")
	case message == "failed to interpret this version number as a semantic version":
		print.Verb("Failed to interpret this version number as a semantic version")
	default:
		if len(message) >= len("failed to interpret latest release tag as a semantic version: ") && message[:len("failed to interpret latest release tag as a semantic version")] == "failed to interpret latest release tag as a semantic version" {
			print.Erro("Failed to interpret latest release tag as a semantic version:", err)
			return
		}
		if len(message) >= len("failed to interpret this version number as a semantic version: ") && message[:len("failed to interpret this version number as a semantic version")] == "failed to interpret this version number as a semantic version" {
			print.Verb("Failed to interpret this version number as a semantic version:", err)
			return
		}
		print.Erro(err)
	}
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
