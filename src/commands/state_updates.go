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
		print.Erro("Failed to check for sampctl updates:", err)
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
		return false, errors.Wrap(err, "invalid sampctl release version")
	}

	current, err := semver.NewVersion(currentVersion)
	if err != nil {
		return false, errors.Wrap(err, "invalid current sampctl version")
	}

	return latest.GreaterThan(current), nil
}

func reportUpgradeCheckError(err error) {
	if err == nil {
		return
	}

	message := err.Error()
	switch {
	case message == "invalid sampctl release version":
		print.Erro("Invalid sampctl release version")
	case message == "invalid current sampctl version":
		print.Verb("Invalid current sampctl version")
	default:
		if len(message) >= len("invalid sampctl release version: ") && message[:len("invalid sampctl release version")] == "invalid sampctl release version" {
			print.Erro(err)
			return
		}
		if len(message) >= len("invalid current sampctl version: ") && message[:len("invalid current sampctl version")] == "invalid current sampctl version" {
			print.Verb(err)
			return
		}
		print.Erro(err)
	}
}

func printUpgradeInstructions(latestVersion, currentVersion string) {
	print.Info("\n-\n")
	print.Info("sampctl", latestVersion, "is available; installed version:", currentVersion)
	print.Info("To upgrade:")

	switch runtime.GOOS {
	case "windows":
		print.Info("  scoop update")
		print.Info("  scoop update sampctl")
	case "linux":
		print.Info("  Debian/Ubuntu:")
		print.Info("  curl https://raw.githubusercontent.com/Southclaws/sampctl/master/scripts/install-deb.sh | sh")
		print.Info("  CentOS/Red Hat:")
		print.Info("  curl https://raw.githubusercontent.com/Southclaws/sampctl/master/scripts/install-rpm.sh | sh")
	case "darwin":
		print.Info("  brew update")
		print.Info("  brew upgrade sampctl")
	}

	print.Info("Report upgrade problems:")
	print.Info("  https://github.com/Southclaws/sampctl/issues/new")
}
