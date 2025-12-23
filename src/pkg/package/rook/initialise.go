package rook

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"text/template"

	"github.com/fatih/color"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/google/go-github/github"
	"github.com/pkg/errors"
	"gopkg.in/AlecAivazis/survey.v1"

	"github.com/Southclaws/sampctl/src/config"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
	"github.com/Southclaws/sampctl/src/pkg/package/pkgcontext"
	"github.com/Southclaws/sampctl/src/pkg/runtime/run"
)

// Answers represents wizard question results
type Answers struct {
	Format        string
	User          string
	Repo          string
	RepoEscaped   string
	PackageType   string
	Runtime       string
	GitIgnore     bool
	Readme        bool
	Editor        string
	StdLib        bool
	Scan          bool
	Git           bool
	EditorConfig  bool
	EntryGenerate bool
	Entry         string
}

// Init prompts the user to initialise a package
func Init(
	ctx context.Context,
	gh *github.Client,
	dir string,
	config *config.Config,
	auth transport.AuthMethod,
	platform,
	cacheDir,
	runtime string,
) (err error) {
	var (
		pwnFiles []string
		incFiles []string
		dirName  = filepath.Base(dir)
	)

	if !fs.Exists(dir) {
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

		switch ext {
		case ".pwn":
			pwnFiles = append(pwnFiles, rel)
		case ".inc":
			incFiles = append(incFiles, rel)
		}

		return
	})
	if err != nil {
		return
	}

	color.Green("Found %d pwn files and %d inc files.", len(pwnFiles), len(incFiles))

	questions := []*survey.Question{
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
			Name: "PackageType",
			Prompt: &survey.Select{
				Message: "Package Type - Are you writing a gamemode or a reusable library?",
				Default: "gamemode",
				Options: []string{"gamemode", "library"},
			},
		},
		{
			Name: "Runtime",
			Prompt: &survey.Select{
				Message: "Target Runtime - Which runtime do you want to use?",
				Default: runtime,
				Options: []string{"samp", "openmp"},
			},
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
			Name:   "EditorConfig",
			Prompt: &survey.Confirm{Message: "Add a .editorconfig file for consistent code formatting?", Default: true},
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

	pkg := pawnpackage.Package{
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

		name := path.Base(nameOnly)
		if name == "/" {
			name = "test"
		}

		pkg.Output = "gamemodes/" + name + ".amx"

		if ext != "" && ext != ".pwn" {
			print.Warn("Entry point is not a .pwn file - it's advised to use a .pwn file as the compiled script.")
			print.Warn("If you are writing a library and not a gamemode or filterscript,")
			print.Warn("it's good to make a separate .pwn file that #includes the .inc file of your library.")
		}

		file := filepath.Join(dir, answers.Entry)
		if !fs.Exists(file) {
			buf := bytes.Buffer{}
			buf.WriteString(`// generated by "sampctl package init"`)
			buf.WriteString("\n")

			if answers.Runtime == "openmp" {
				buf.WriteString(`#include <open.mp>`)
			} else {
				buf.WriteString(`#include <a_samp>`)
			}

			buf.WriteString("\n")
			buf.WriteString("\nmain() \n{\n")
			buf.WriteString(`	// write code here and run "sampctl package build" to compile`)
			buf.WriteString("\n")
			buf.WriteString(`	// then run "sampctl package run" to run it`)
			buf.WriteString("\n}\n")
			err := ioutil.WriteFile(file, buf.Bytes(), 0o600)
			if err != nil {
				color.Red("failed to write generated %s entry file: %v", answers.Entry, err)
			}
		}
	} else {
		if answers.EntryGenerate {
			buf := bytes.Buffer{}

			buf.WriteString(`// generated by "sampctl package generate"`)
			buf.WriteString("\n")

			if answers.Runtime == "openmp" {
				buf.WriteString(`#include <open.mp>`)
			} else {
				buf.WriteString(`#include <a_samp>`)
			}
			buf.WriteString("\n")

			for _, inc := range incFiles {
				buf.WriteString(fmt.Sprintf(`#include "%s"%s`, filepath.Base(inc), "\n"))
			}
			buf.WriteString("\nmain() \n{\n")
			buf.WriteString(`	// write tests for libraries here and run "sampctl package run"`)
			buf.WriteString("\n}\n")
			err = ioutil.WriteFile(filepath.Join(dir, "test.pwn"), buf.Bytes(), 0o600)
			if err != nil {
				color.Red("failed to write generated tests.pwn file: %v", err)
			}
		}
		pkg.Entry = "test.pwn"
		pkg.Output = "gamemodes/test.amx"
	}

	if answers.PackageType == "gamemode" {
		pkg.Local = true
	} else {
		pkg.Local = false
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
		if answers.Runtime == "openmp" {
			pkg.Dependencies = append(pkg.Dependencies, versioning.DependencyString("openmultiplayer/omp-stdlib"))
		} else {
			pkg.Dependencies = append(pkg.Dependencies, versioning.DependencyString("pawn-lang/samp-stdlib"))
		}
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

	if answers.EditorConfig {
		wg.Add(1)
		go func() {
			errInner := createEditorConfig(dir)
			if errInner != nil {
				print.Erro("Failed to create .editorconfig:", errInner)
			}
			wg.Done()
		}()
	}

	if pkg.Runtime == nil {
		pkg.Runtime = &run.Runtime{}
	}

	if answers.Runtime == "openmp" {
		pkg.Runtime.Version = "openmp"
		pkg.Runtime.Plugins = []run.Plugin{}

		os.MkdirAll(filepath.Join(dir, "gamemodes"), 0o755)
		os.MkdirAll(filepath.Join(dir, "filterscripts"), 0o755)
		os.MkdirAll(filepath.Join(dir, "include"), 0o755)
		if err := fs.EnsurePackageLayout(dir, true); err != nil {
			return err
		}
	} else {
		pkg.Runtime.Version = "0.3.7"
		pkg.Runtime.Plugins = []run.Plugin{}

		os.MkdirAll(filepath.Join(dir, "gamemodes"), 0o755)
		os.MkdirAll(filepath.Join(dir, "filterscripts"), 0o755)
		os.MkdirAll(filepath.Join(dir, "include"), 0o755)
		if err := fs.EnsurePackageLayout(dir, false); err != nil {
			return err
		}
		os.MkdirAll(filepath.Join(dir, "npcmodes"), 0o755)
	}

	// add a default tag
	pkg.Tag = "0.0.1"

	err = pkg.WriteDefinition()
	if err != nil {
		print.Erro(err)
	}

	wg.Wait()

	pcx, err := pkgcontext.NewPackageContext(gh, auth, true, dir, platform, cacheDir, "", true)
	if err != nil {
		return
	}
	err = pcx.EnsureDependenciesWithRuntime(ctx, true, false)
	if err != nil {
		return
	}

	return nil
}

func createEditorConfig(dir string) error {
	editorConfigContent := `root = true

[*.{pwn,inc}]
indent_style = tab
indent_size = 4
end_of_line = lf
insert_final_newline = true
charset = utf-8
`
	outputFile := filepath.Join(dir, ".editorconfig")
	if fs.Exists(outputFile) {
		return nil // Don't overwrite existing file
	}

	err := ioutil.WriteFile(outputFile, []byte(editorConfigContent), 0o644)
	if err != nil {
		return errors.Wrap(err, "failed to write .editorconfig")
	}

	return nil
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

	if fs.Exists(outputFile) {
		outputFile = outputFile + "-duplicate"
	}

	err = os.MkdirAll(filepath.Dir(outputFile), 0o700)
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
	if err != nil {
		return
	}

	return nil
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
