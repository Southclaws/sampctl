package rook

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"text/template"

	"github.com/fatih/color"
	"github.com/google/go-github/github"
	"github.com/pkg/errors"
	"gopkg.in/AlecAivazis/survey.v1"
	"gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing/transport"

	"github.com/Southclaws/sampctl/print"
	"github.com/Southclaws/sampctl/types"
	"github.com/Southclaws/sampctl/util"
	"github.com/Southclaws/sampctl/versioning"
)

// Answers represents wizard question results
type Answers struct {
	Format        string
	User          string
	Repo          string
	RepoEscaped   string
	GitIgnore     bool
	Readme        bool
	Editor        string
	StdLib        bool
	Scan          bool
	Git           bool
	Travis        bool
	EntryGenerate bool
	Entry         string
}

// Init prompts the user to initialise a package
func Init(ctx context.Context, gh *github.Client, dir string, config *types.Config, auth transport.AuthMethod, platform, cacheDir string) (err error) {
	var (
		pwnFiles []string
		incFiles []string
		dirName  = filepath.Base(dir)
	)

	if !util.Exists(dir) {
		return errors.New("directory does not exist")
	}

	err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) (innerErr error) {
		if info.IsDir() {
			return nil
		}

		// skip anything in dependencies
		base, errInner := filepath.Rel(dir, path)
		if errInner != nil {
			return errInner
		}
		if strings.Contains(filepath.Dir(base), "dependencies") {
			return nil
		}

		ext := filepath.Ext(path)
		rel, innerErr := filepath.Rel(dir, path)
		if innerErr != nil {
			return
		}

		if ext == ".pwn" {
			pwnFiles = append(pwnFiles, rel)
		} else if ext == ".inc" {
			incFiles = append(incFiles, rel)
		}

		return
	})
	if err != nil {
		return
	}

	color.Green("Found %d pwn files and %d inc files.", len(pwnFiles), len(incFiles))

	var questions = []*survey.Question{
		{
			Name: "Format",
			Prompt: &survey.Select{
				Message: "Preferred package format",
				Options: []string{"json", "yaml"},
			},
			Validate: survey.Required,
		},
		{
			Name: "User",
			Prompt: &survey.Input{
				Message: "Your Name - If you plan to release, must be your GitHub username.",
				Default: config.DefaultUser,
			},
			Validate: validateUser,
		},
		{
			Name: "Repo",
			Prompt: &survey.Input{
				Message: "Package Name - If you plan to release, must be the GitHub project name.",
				Default: dirName,
			},
			Validate: validateRepo,
		},
		{
			Name:   "GitIgnore",
			Prompt: &survey.Confirm{Message: "Add a .gitignore and .gitattributes files?", Default: true},
		},
		{
			Name:   "Readme",
			Prompt: &survey.Confirm{Message: "Add a README.md file?", Default: true},
		},
		{
			Name: "Editor",
			Prompt: &survey.Select{
				Message: "Select your text editor",
				Options: []string{"none", "vscode", "sublime"},
			},
			Validate: survey.Required,
		},
		{
			Name:   "StdLib",
			Prompt: &survey.Confirm{Message: "Add standard library dependency?", Default: true},
		},
		{
			Name:   "Scan",
			Prompt: &survey.Confirm{Message: "Scan for dependencies?", Default: true},
		},
		{
			Name:   "Git",
			Prompt: &survey.Confirm{Message: "Initialise a git repository?", Default: true},
		},
		{
			Name:   "Travis",
			Prompt: &survey.Confirm{Message: "Add a .travis.yml for unit testing?", Default: false},
		},
	}

	if len(pwnFiles) > 0 {
		questions = append(questions, &survey.Question{
			Name: "Entry",
			Prompt: &survey.Select{
				Message: "Choose an entry point - this is the file that is passed to the compiler.",
				Options: pwnFiles,
			},
			Validate: survey.Required,
		})
	} else {
		if len(incFiles) > 0 {
			questions = append(questions, &survey.Question{
				Name: "EntryGenerate",
				Prompt: &survey.Confirm{
					Message: "No .pwn found but .inc found - create .pwn file that includes .inc?",
					Default: true,
				},
			})
		} else {
			questions = append(questions, &survey.Question{
				Name: "Entry",
				Prompt: &survey.Input{
					Message: "No .pwn or .inc files - enter name for new script",
					Default: "test.pwn",
				},
			})
		}
	}

	answers := Answers{}
	err = survey.Ask(questions, &answers)
	if err != nil {
		return
	}

	if answers.User != config.DefaultUser {
		config.DefaultUser = answers.User
	}

	pkg := types.Package{
		Parent:    true,
		LocalPath: dir,
		Format:    answers.Format,
		DependencyMeta: versioning.DependencyMeta{
			User: answers.User,
			Repo: answers.Repo,
		},
	}

	if answers.Entry != "" {
		ext := filepath.Ext(answers.Entry)
		nameOnly := strings.TrimSuffix(answers.Entry, ext)
		pkg.Entry = nameOnly + ".pwn"
		pkg.Output = nameOnly + ".amx"

		if ext != "" && ext != ".pwn" {
			print.Warn("Entry point is not a .pwn file - it's advised to use a .pwn file as the compiled script.")
			print.Warn("If you are writing a library and not a gamemode or filterscript,")
			print.Warn("it's good to make a separate .pwn file that #includes the .inc file of your library.")
		}
	} else {
		if answers.EntryGenerate {
			buf := bytes.Buffer{}

			buf.WriteString(`// generated by "sampctl package generate"`)
			buf.WriteString("\n\n")
			for _, inc := range incFiles {
				buf.WriteString(fmt.Sprintf(`#include "%s"%s`, filepath.Base(inc), "\n"))
			}
			buf.WriteString("\nmain() {\n")
			buf.WriteString(`	// write tests for libraries here and run "sampctl package run"`)
			buf.WriteString("\n}\n")
			err = ioutil.WriteFile(filepath.Join(dir, "test.pwn"), buf.Bytes(), 0600)
			if err != nil {
				color.Red("failed to write generated tests.pwn file: %v", err)
			}
		}
		pkg.Entry = "test.pwn"
		pkg.Output = "test.amx"
	}

	wg := sync.WaitGroup{}

	if answers.GitIgnore {
		wg.Add(1)
		go func() {
			errInner := getTemplateFile(dir, ".gitignore", answers)
			if errInner != nil {
				print.Erro("Failed to get .gitignore template:", errInner)
			}
			wg.Done()
		}()
		wg.Add(1)
		go func() {
			errInner := getTemplateFile(dir, ".gitattributes", answers)
			if errInner != nil {
				print.Erro("Failed to get .gitattributes template:", errInner)
			}
			wg.Done()
		}()
	}

	if answers.Readme {
		wg.Add(1)
		go func() {
			errInner := getTemplateFile(dir, "README.md", answers)
			if err != nil {
				print.Erro("Failed to get readme template:", errInner)
				return
			}
			defer wg.Done()
		}()
	}

	switch answers.Editor {
	case "vscode":
		wg.Add(1)
		go func() {
			errInner := getTemplateFile(dir, ".vscode/tasks.json", answers)
			if errInner != nil {
				print.Erro("Failed to get tasks.json template:", errInner)
			}
			wg.Done()
		}()
	case "sublime":
		wg.Add(1)
		go func() {
			errInner := getTemplateFile(dir, "{{.Repo}}.sublime-project", answers)
			if errInner != nil {
				print.Erro("Failed to get tasks.json template:", errInner)
			}
			wg.Done()
		}()
	}

	if answers.StdLib {
		pkg.Dependencies = append(pkg.Dependencies, versioning.DependencyString("sampctl/samp-stdlib"))
	}

	if answers.Scan {
		pkg.Dependencies = append(pkg.Dependencies, FindIncludes(incFiles)...)
	}

	if answers.Git {
		_, err = git.PlainInit(dir, false)
		if err != nil {
			print.Erro("Failed to initialise git repo:", err)
		}
		print.Info("You can use `sampctl package release` to apply a version number and release your first version!")
	}

	if answers.Travis {
		pkg.Runtime = &types.Runtime{Mode: "y_testing"}
		wg.Add(1)
		go func() {
			errInner := getTemplateFile(dir, ".travis.yml", answers)
			if errInner != nil {
				print.Erro("Failed to get .travis.yml template:", errInner)
			}
			wg.Done()
		}()
	}

	err = pkg.WriteDefinition()
	if err != nil {
		print.Erro(err)
	}

	wg.Wait()

	pcx, err := NewPackageContext(gh, auth, true, dir, platform, cacheDir, "")
	if err != nil {
		return
	}
	err = pcx.EnsureDependencies(ctx, true)

	return
}

func getTemplateFile(dir, filename string, answers Answers) (err error) {
	resp, err := http.Get("https://raw.githubusercontent.com/Southclaws/pawn-package-template/master/" + filename)
	if err != nil {
		return
	}
	defer func() {
		errDefer := resp.Body.Close()
		if errDefer != nil {
			panic(errDefer)
		}
	}()

	contents, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return
	}

	outputContents, errInner := doTemplate(string(contents), answers)
	if errInner != nil {
		return
	}

	outputFile, err := doTemplate(filepath.Join(dir, filename), answers)
	if err != nil {
		return
	}

	if util.Exists(outputFile) {
		outputFile = outputFile + "-duplicate"
	}

	err = os.MkdirAll(filepath.Dir(outputFile), 0700)
	if err != nil {
		return
	}

	file, err := os.Create(outputFile)
	if err != nil {
		return
	}
	defer func() {
		err = file.Close()
		if err != nil {
			print.Erro(err)
		}
	}()

	_, err = file.WriteString(outputContents)

	return
}

func validateUser(ans interface{}) (err error) {
	if strings.ContainsAny(ans.(string), ` :;/\\~`) {
		return errors.New("Contains invalid characters")
	}
	return
}

func validateRepo(ans interface{}) (err error) {
	if strings.ContainsAny(ans.(string), ` :;/\\~`) {
		return errors.New("Contains invalid characters")
	}
	return
}

func doTemplate(input string, answers Answers) (output string, err error) {
	output = input // for error returns
	out := &bytes.Buffer{}

	tmpl, err := template.New("tmp").Parse(input)
	if err != nil {
		err = errors.Wrap(err, "failed to parse input as template")
		return
	}

	answers.RepoEscaped = strings.Replace(answers.Repo, "-", "--", -1)
	err = tmpl.Execute(out, answers)
	if err != nil {
		err = errors.Wrap(err, "failed to execute template")
		return
	}

	output = out.String()
	return
}
