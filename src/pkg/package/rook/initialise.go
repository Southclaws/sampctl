package rook

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/user"
	"path"
	"path/filepath"
	"sort"
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
)

// Answers represents wizard question results
type Answers struct {
	User        string
	Repo        string
	RepoEscaped string
	InitMode    string
	Preset      string
	Starter     string
	Format      string
	PublishMode string
	Repository  string
	Entry       string
}

const (
	defaultInitFormat = "json"

	initModeUsePwn      = "Use an existing .pwn entry file"
	initModeUseInc      = "Use existing .inc files and create a test harness"
	initModeNewGamemode = "Create new gamemode"
	initModeNewHarness  = "Create new library"

	starterMinimal  = "Minimal"
	starterStandard = "Standard (Recommended)"
	starterVSCode   = "Standard + VSCode"
	starterSublime  = "Standard + Sublime"

	runtimeOptionOpenMP = "open.mp (Recommended)"
	runtimeOptionSAMP   = "SA-MP"

	formatOptionJSON = "JSON (Recommended)"
	formatOptionYAML = "YAML"

	publishLocal  = "No"
	publishGitHub = "Yes"

	defaultGamemodeEntry = "main.pwn"
	defaultHarnessEntry  = "test.pwn"

	localReadmeTemplate = `# {{.Repo}}

This package was created with ` + "`sampctl init`" + `.

## Build

` + "```bash" + `
sampctl build
` + "```" + `

## Run

` + "```bash" + `
sampctl run
` + "```" + `

## Publish Later

If you decide to publish this package on GitHub later, update the package owner/repo fields and refresh this README before releasing.
`
)

type starterProfile struct {
	GitIgnore    bool
	Readme       bool
	Editor       string
	Git          bool
	EditorConfig bool
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
	preset,
	version string,
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

	sort.Strings(pwnFiles)
	sort.Strings(incFiles)

	color.Green("Found %d pwn files and %d inc files.", len(pwnFiles), len(incFiles))

	answers := Answers{
		Preset: normalizePreset(preset),
		Format: defaultInitFormat,
	}

	initModes := initModeOptions(pwnFiles, incFiles)
	err = survey.AskOne(&survey.Select{
		Message: "How do you want to start this package?",
		Options: initModes,
		Default: initModes[0],
	}, &answers.InitMode, survey.Required)
	if err != nil {
		return
	}

	runtimeChoice := runtimeChoiceFromPreset(preset)
	err = survey.AskOne(&survey.Select{
		Message: "Which runtime should this package target?",
		Options: []string{runtimeOptionOpenMP, runtimeOptionSAMP},
		Default: runtimeChoice,
	}, &runtimeChoice, survey.Required)
	if err != nil {
		return
	}
	answers.Preset = presetFromRuntimeChoice(runtimeChoice)

	err = survey.AskOne(&survey.Select{
		Message: "Which starter setup do you want?",
		Options: []string{starterStandard, starterVSCode, starterSublime, starterMinimal},
		Default: starterStandard,
	}, &answers.Starter, survey.Required)
	if err != nil {
		return
	}

	formatChoice := formatChoiceFromFormat(defaultInitFormat)
	err = survey.AskOne(&survey.Select{
		Message: "Which package file format do you want?",
		Options: []string{formatOptionJSON, formatOptionYAML},
		Default: formatChoice,
	}, &formatChoice, survey.Required)
	if err != nil {
		return
	}
	answers.Format = formatFromChoice(formatChoice)

	publishChoice := publishChoiceFromMode(answers.PublishMode)
	err = survey.AskOne(&survey.Select{
		Message: "Do you plan to release this as a package on GitHub?",
		Options: []string{publishLocal, publishGitHub},
		Default: publishChoice,
	}, &publishChoice, survey.Required)
	if err != nil {
		return
	}
	answers.PublishMode = publishModeFromChoice(publishChoice)

	if answers.PublishMode == publishGitHub {
		err = survey.AskOne(&survey.Input{
			Message: "GitHub repository to publish from (owner/repo)",
			Default: defaultRepositorySpec(config.DefaultUser, dirName),
		}, &answers.Repository, validateRepositorySpec)
		if err != nil {
			return
		}
	}

	if answers.InitMode == initModeUsePwn {
		if len(pwnFiles) == 1 {
			answers.Entry = pwnFiles[0]
		} else {
			err = survey.AskOne(&survey.Select{
				Message: "Which existing .pwn file should sampctl build?",
				Options: pwnFiles,
				Default: pwnFiles[0],
			}, &answers.Entry, survey.Required)
			if err != nil {
				return
			}
		}
	}

	metadataUser := defaultMetadataUser(config.DefaultUser)
	metadataRepo := defaultRepoName(dirName)
	templateUser := metadataUser
	if answers.PublishMode == publishGitHub {
		metadataUser, metadataRepo, err = splitRepositorySpec(answers.Repository)
		if err != nil {
			return
		}
		templateUser = metadataUser
		if metadataUser != config.DefaultUser {
			config.DefaultUser = metadataUser
		}
	}

	answers.User = templateUser
	answers.Repo = metadataRepo

	pkg := pawnpackage.Package{
		Parent:    true,
		LocalPath: dir,
		Format:    answers.Format,
		DependencyMeta: versioning.DependencyMeta{
			User: metadataUser,
			Repo: metadataRepo,
		},
	}

	pkg.Preset = answers.Preset
	profile := starterProfileFor(answers.Starter)

	switch answers.InitMode {
	case initModeUsePwn:
		pkg.Entry = filepath.ToSlash(answers.Entry)
		nameOnly := strings.TrimSuffix(pkg.Entry, filepath.Ext(pkg.Entry))
		name := path.Base(nameOnly)
		if name == "." || name == "/" || name == "" {
			name = "test"
		}
		pkg.Output = "gamemodes/" + name + ".amx"
	case initModeUseInc, initModeNewHarness:
		pkg.Entry = defaultHarnessEntry
		pkg.Output = "gamemodes/test.amx"
		err = writeGeneratedEntryFile(filepath.Join(dir, defaultHarnessEntry), generatedHarnessContents(answers.Preset, incFiles))
		if err != nil {
			return errors.Wrap(err, "failed to write generated test harness")
		}
	case initModeNewGamemode:
		pkg.Entry = defaultGamemodeEntry
		pkg.Output = "gamemodes/main.amx"
		err = writeGeneratedEntryFile(filepath.Join(dir, defaultGamemodeEntry), generatedGamemodeContents(answers.Preset))
		if err != nil {
			return errors.Wrap(err, "failed to write generated entry file")
		}
	default:
		return errors.Errorf("unsupported init mode: %s", answers.InitMode)
	}

	wg := sync.WaitGroup{}

	if profile.GitIgnore {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errInner := getTemplateFile(ctx, dir, ".gitignore", answers)
			if errInner != nil {
				print.Erro("Failed to get .gitignore template:", errInner)
			}
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			errInner := getTemplateFile(ctx, dir, ".gitattributes", answers)
			if errInner != nil {
				print.Erro("Failed to get .gitattributes template:", errInner)
			}
		}()
	}

	if profile.Readme {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errInner := getTemplateFile(ctx, dir, "README.md", answers)
			if errInner != nil {
				print.Erro("Failed to get readme template:", errInner)
			}
		}()
	}

	switch profile.Editor {
	case "vscode":
		wg.Add(1)
		go func() {
			defer wg.Done()
			errInner := getTemplateFile(ctx, dir, ".vscode/tasks.json", answers)
			if errInner != nil {
				print.Erro("Failed to get tasks.json template:", errInner)
			}
		}()
	case "sublime":
		wg.Add(1)
		go func() {
			defer wg.Done()
			errInner := getTemplateFile(ctx, dir, "{{.Repo}}.sublime-project", answers)
			if errInner != nil {
				print.Erro("Failed to get tasks.json template:", errInner)
			}
		}()
	}

	pkg.Dependencies = appendUniqueDependencies(pkg.Dependencies, stdDependenciesForPreset(answers.Preset)...)
	pkg.Dependencies = appendUniqueDependencies(pkg.Dependencies, detectedIncludeDependencies(dir, incFiles)...)

	if profile.Git && !fs.Exists(filepath.Join(dir, ".git")) {
		_, err = git.PlainInit(dir, false)
		if err != nil {
			print.Erro("Failed to initialise git repo:", err)
		}
		if releaseHint := releaseHintForPublishMode(answers.PublishMode); releaseHint != "" {
			print.Info(releaseHint)
		}
	}

	if profile.EditorConfig {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errInner := getTemplateFile(ctx, dir, ".editorconfig", answers)
			if errInner != nil {
				print.Erro("Failed to get .editorconfig template:", errInner)
			}
		}()
	}

	if err := fs.EnsurePackageLayout(dir, answers.Preset == "openmp"); err != nil {
		return err
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
	err = pcx.InitLockfileResolver(version)
	if err != nil {
		return errors.Wrap(err, "failed to initialize lockfile resolver")
	}

	_, err = pcx.EnsureProject(ctx, true)
	if err != nil {
		return
	}

	return nil
}

func initModeOptions(pwnFiles, incFiles []string) []string {
	options := make([]string, 0, 4)

	if len(pwnFiles) > 0 {
		options = append(options, initModeUsePwn)
	} else if len(incFiles) > 0 {
		options = append(options, initModeUseInc)
	}

	options = append(options, initModeNewGamemode, initModeNewHarness)
	return options
}

func normalizePreset(preset string) string {
	if strings.EqualFold(strings.TrimSpace(preset), "samp") {
		return "samp"
	}

	return "openmp"
}

func runtimeChoiceFromPreset(preset string) string {
	if normalizePreset(preset) == "samp" {
		return runtimeOptionSAMP
	}

	return runtimeOptionOpenMP
}

func presetFromRuntimeChoice(choice string) string {
	if choice == runtimeOptionSAMP || strings.EqualFold(choice, "samp") {
		return "samp"
	}

	return "openmp"
}

func formatChoiceFromFormat(format string) string {
	if strings.EqualFold(strings.TrimSpace(format), "yaml") {
		return formatOptionYAML
	}

	return formatOptionJSON
}

func formatFromChoice(choice string) string {
	if choice == formatOptionYAML || strings.EqualFold(choice, "yaml") {
		return "yaml"
	}

	return "json"
}

func publishChoiceFromMode(mode string) string {
	if mode == publishGitHub {
		return publishGitHub
	}

	return publishLocal
}

func publishModeFromChoice(choice string) string {
	if choice == publishGitHub {
		return publishGitHub
	}

	return publishLocal
}

func releaseHintForPublishMode(mode string) string {
	if mode == publishGitHub {
		return "You can use `sampctl release` to apply a version number and release your first version!"
	}

	return ""
}

func starterProfileFor(choice string) starterProfile {
	switch choice {
	case starterMinimal:
		return starterProfile{}
	case starterVSCode:
		return starterProfile{
			GitIgnore:    true,
			Readme:       true,
			Editor:       "vscode",
			Git:          true,
			EditorConfig: true,
		}
	case starterSublime:
		return starterProfile{
			GitIgnore:    true,
			Readme:       true,
			Editor:       "sublime",
			Git:          true,
			EditorConfig: true,
		}
	case starterStandard:
		fallthrough
	default:
		return starterProfile{
			GitIgnore:    true,
			Readme:       true,
			Git:          true,
			EditorConfig: true,
		}
	}
}

func defaultMetadataUser(defaultUser string) string {
	if trimmed := strings.TrimSpace(defaultUser); trimmed != "" {
		return trimmed
	}

	currentUser, err := user.Current()
	if err == nil {
		if trimmed := strings.TrimSpace(currentUser.Username); trimmed != "" {
			return trimmed
		}
	}

	return "local-user"
}

func defaultRepoName(dirName string) string {
	cleaned := strings.TrimSpace(dirName)
	if cleaned == "" {
		return "package"
	}

	cleaned = strings.ReplaceAll(cleaned, " ", "-")
	cleaned = strings.Map(func(r rune) rune {
		switch r {
		case ':', ';', '/', '\\', '~':
			return -1
		default:
			return r
		}
	}, cleaned)
	cleaned = strings.Trim(cleaned, "-")
	if cleaned == "" {
		return "package"
	}

	return cleaned
}

func defaultRepositorySpec(defaultUser, dirName string) string {
	user := strings.TrimSpace(defaultUser)
	if user == "" {
		user = "your-github-user"
	}

	return fmt.Sprintf("%s/%s", user, defaultRepoName(dirName))
}

func splitRepositorySpec(spec string) (user, repo string, err error) {
	trimmed := strings.TrimSpace(spec)
	parts := strings.Split(trimmed, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", errors.New("repository must be in the format user/repo")
	}

	if err = validateUser(parts[0]); err != nil {
		return "", "", errors.Wrap(err, "invalid user")
	}
	if err = validateRepo(parts[1]); err != nil {
		return "", "", errors.Wrap(err, "invalid repo")
	}

	return parts[0], parts[1], nil
}

func validateRepositorySpec(ans any) error {
	_, _, err := splitRepositorySpec(ans.(string))
	return err
}

func runtimeIncludeForPreset(preset string) string {
	if preset == "openmp" {
		return "<open.mp>"
	}

	return "<a_samp>"
}

func generatedGamemodeContents(preset string) []byte {
	buf := bytes.Buffer{}
	buf.WriteString(`// generated by "sampctl init"`)
	buf.WriteString("\n")
	buf.WriteString(fmt.Sprintf("#include %s\n\n", runtimeIncludeForPreset(preset)))
	buf.WriteString("main()\n{\n")
	buf.WriteString("\t// write code here and run \"sampctl build\" to compile\n")
	buf.WriteString("\t// then run \"sampctl run\" to run it\n")
	buf.WriteString("}\n")
	return buf.Bytes()
}

func generatedHarnessContents(preset string, incFiles []string) []byte {
	buf := bytes.Buffer{}
	buf.WriteString(`// generated by "sampctl init"`)
	buf.WriteString("\n")
	buf.WriteString(fmt.Sprintf("#include %s\n", runtimeIncludeForPreset(preset)))

	for _, inc := range incFiles {
		buf.WriteString(fmt.Sprintf("#include %q\n", filepath.ToSlash(inc)))
	}

	buf.WriteString("\nmain()\n{\n")
	buf.WriteString("\t// write tests for libraries here and run \"sampctl run\"\n")
	buf.WriteString("}\n")
	return buf.Bytes()
}

func writeGeneratedEntryFile(path string, contents []byte) error {
	if fs.Exists(path) {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}

	return os.WriteFile(path, contents, 0o600)
}

func stdDependenciesForPreset(preset string) []versioning.DependencyString {
	if preset == "openmp" {
		return []versioning.DependencyString{
			versioning.DependencyString("openmultiplayer/omp-stdlib"),
			versioning.DependencyString("pawn-lang/samp-stdlib@open.mp"),
			versioning.DependencyString("pawn-lang/pawn-stdlib@open.mp"),
		}
	}

	return []versioning.DependencyString{
		versioning.DependencyString("pawn-lang/samp-stdlib"),
	}
}

func detectedIncludeDependencies(dir string, incFiles []string) []versioning.DependencyString {
	if len(incFiles) == 0 {
		return nil
	}

	files := make([]string, 0, len(incFiles))
	for _, file := range incFiles {
		files = append(files, filepath.Join(dir, file))
	}

	return FindIncludes(files)
}

func appendUniqueDependencies(dst []versioning.DependencyString, deps ...versioning.DependencyString) []versioning.DependencyString {
	seen := make(map[versioning.DependencyString]struct{}, len(dst)+len(deps))
	for _, dep := range dst {
		seen[dep] = struct{}{}
	}

	for _, dep := range deps {
		if _, ok := seen[dep]; ok {
			continue
		}
		dst = append(dst, dep)
		seen[dep] = struct{}{}
	}

	return dst
}

func getTemplateFile(ctx context.Context, dir, filename string, answers Answers) (err error) {
	contents, err := templateFileContents(ctx, filename, answers)
	if err != nil {
		return err
	}

	outputContents, errInner := doTemplate(contents, answers)
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
		if errClose := file.Close(); errClose != nil {
			if err == nil {
				err = errors.Wrap(errClose, "failed to close template file")
				return
			}
			print.Warn("failed to close template file:", errClose)
		}
	}()

	_, err = file.WriteString(outputContents)
	if err != nil {
		return
	}

	return nil
}

func templateFileContents(ctx context.Context, filename string, answers Answers) (string, error) {
	if filename == "README.md" && answers.PublishMode == publishLocal {
		return localReadmeTemplate, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://raw.githubusercontent.com/sampctl/pawn-package-template/master/"+filename, nil)
	if err != nil {
		return "", err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() {
		if errClose := resp.Body.Close(); errClose != nil {
			print.Warn("failed to close template response body:", errClose)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return "", errors.Errorf("failed to download template %s: HTTP %d", filename, resp.StatusCode)
	}

	contents, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(contents), nil
}

func validateUser(ans any) (err error) {
	if strings.ContainsAny(ans.(string), ` :;/\\~`) {
		return errors.New("Contains invalid characters")
	}
	return
}

func validateRepo(ans any) (err error) {
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
