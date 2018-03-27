package rook

import (
	"fmt"
	"sort"
	"strings"

	"github.com/pkg/errors"
	"gopkg.in/AlecAivazis/survey.v1"
	"gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"

	"github.com/Southclaws/sampctl/print"
	"github.com/Southclaws/sampctl/types"
	"github.com/Southclaws/sampctl/versioning"
)

// Release is an interactive release tool for package versioning
func Release(pkg types.Package) (err error) {
	repo, err := git.PlainOpen(pkg.Local)
	if err != nil {
		return errors.Wrap(err, "failed to read package as git repository")
	}

	head, err := repo.Head()
	if err != nil {
		return errors.Wrap(err, "failed to get repo HEAD reference")
	}

	tags, err := versioning.GetRepoSemverTags(repo)
	if err != nil {
		return errors.Wrap(err, "failed to get semver tags")
	}
	sort.Sort(sort.Reverse(tags))

	var questions []*survey.Question
	var answers struct{ Version string, Distribution bool, GitHub bool }

	if len(tags) == 0 {
		questions = []*survey.Question{
			{
				Name: "Version",
				Prompt: &survey.Select{
					Message: "New Project Version",
					Options: []string{
						"0.0.1: Unstable prototype",
						"0.1.0: Stable prototype but subject to change",
						"1.0.0: Stable release, API won't change",
					},
				},
				Validate: survey.Required,
			},
		}
	} else {
		var latest versioning.VersionedTag = tags[0]

		print.Info("Latest version:", latest.Tag)

		bumpPatch := latest.Tag.IncPatch()
		bumpMinor := latest.Tag.IncMinor()
		bumpMajor := latest.Tag.IncMajor()

		questions = []*survey.Question{
			{
				Name: "Version",
				Prompt: &survey.Select{
					Message: "Select Version Bump",
					Options: []string{
						fmt.Sprintf("%s: I made backwards-compatible bug fixes", bumpPatch.String()),
						fmt.Sprintf("%s: I added functionality in a backwards-compatible manner", bumpMinor.String()),
						fmt.Sprintf("%s: I made incompatible API changes", bumpMajor.String()),
					},
				},
				Validate: survey.Required,
			},
		}
	}

	err = survey.Ask(questions, &answers)
	if err != nil {
		return errors.Wrap(err, "failed to open wizard")
	}

	print.Info("New version:", answers.Version)

	newVersion := strings.Split(answers.Version, ":")[0]

	ref := plumbing.ReferenceName("refs/tags/" + newVersion)
	hash := plumbing.NewHashReference(ref, head.Hash())
	err = repo.Storer.SetReference(hash)

	return
}
