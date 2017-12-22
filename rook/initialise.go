package rook

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/fatih/color"
	"gopkg.in/AlecAivazis/survey.v1"
)

// Init prompts the user to initialise a package
func Init(dir string) (err error) {
	var (
		pwnFiles []string
		incFiles []string
	)

	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if info.IsDir() {
			return nil
		}

		ext := filepath.Ext(path)

		if ext == ".pwn" {
			pwnFiles = append(pwnFiles, path)
		} else if ext == ".inc" {
			incFiles = append(incFiles, path)
		}

		return nil
	})

	color.Green("Found %d pwn files and %d inc files.", len(pwnFiles), len(incFiles))

	var questions = []*survey.Question{
		{
			Name:     "User",
			Prompt:   &survey.Input{Message: "Your Name - If you plan to release, must be your GitHub username."},
			Validate: survey.Required,
		},
		{
			Name:     "Repo",
			Prompt:   &survey.Input{Message: "Package Name - If you plan to release, must be the GitHub project name."},
			Validate: survey.Required,
		},
	}

	if len(pwnFiles) == 0 {
		if len(incFiles) > 0 {
			questions = append(questions, &survey.Question{
				Name: "EntryGenerate",
				Prompt: &survey.MultiSelect{
					Message: "No .pwn found but .inc found - create .pwn file that includes .inc?",
					Options: incFiles,
				},
				Validate: survey.Required,
			})
		}
	} else {
		questions = append(questions, &survey.Question{
			Name: "EntryChoose",
			Prompt: &survey.Select{
				Message: "Choose an entry point - this is the file that is passed to the compiler.",
				Options: pwnFiles,
			},
			Validate: survey.Required,
		})
	}

	answers := struct {
		User          string
		Repo          string
		EntryGenerate []string
		EntryChoose   []string
	}{}

	err = survey.Ask(questions, &answers)
	if err != nil {
		return
	}

	fmt.Println(answers)

	return
}
