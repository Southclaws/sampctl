package runtime

import (
	"io/ioutil"
	"path/filepath"
	"strings"

	garbler "github.com/michaelbironneau/garbler/lib"
	zxcvbn "github.com/nbutton23/zxcvbn-go"
	"github.com/pkg/errors"
	"gopkg.in/AlecAivazis/survey.v1"

	"github.com/Southclaws/sampctl/print"
	"github.com/Southclaws/sampctl/types"
	"github.com/Southclaws/sampctl/util"
)

// InitialiseServer creates a samp.json by asking the user a series of questions
// nolint:gocyclo
func InitialiseServer(version, dir, platform string) (err error) {
	var (
		gamemodesDir      = filepath.Join(dir, "gamemodes")
		filterscriptsDir  = filepath.Join(dir, "filterscripts")
		pluginsDir        = filepath.Join(dir, "plugins")
		gamemodesList     []string
		filterscriptsList []string
		pluginsList       []string
	)

	if !util.Exists(gamemodesDir) {
		//nolint:lll
		print.Warn("This directory does not appear to have a gamemodes directory, you must add at least one gamemode to run a server")
	} else {
		gamemodesList = getAmxFiles(gamemodesDir)
	}

	if !util.Exists(filterscriptsDir) {
		print.Warn("This directory does not appear to have a filterscripts directory")
	} else {
		filterscriptsList = getAmxFiles(filterscriptsDir)
	}

	if !util.Exists(pluginsDir) {
		print.Warn("This directory does not appear to have a plugins directory")
	} else {
		pluginsList = getPlugins(pluginsDir, platform)
	}

	var questions = []*survey.Question{
		{
			Name: "Format",
			Prompt: &survey.Select{
				Message: "Preferred configuration format",
				Options: []string{"json", "yaml"},
			},
			Validate: survey.Required,
		},
		{
			Name:     "Hostname",
			Prompt:   &survey.Input{Message: "Server Hostname"},
			Validate: survey.Required,
		},
		{
			Name:   "RCONPassword",
			Prompt: &survey.Input{Message: "RCON Password (leave blank to generate a strong one)"},
		},
		{
			Name:   "Port",
			Prompt: &survey.Input{Message: "Server Port", Default: "7777"},
		},
		{
			Name:   "MaxPlayers",
			Prompt: &survey.Input{Message: "Max Players", Default: "32"},
		},
	}

	if len(gamemodesList) > 0 {
		questions = append(questions, &survey.Question{
			Name: "Gamemodes",
			Prompt: &survey.MultiSelect{
				Message: "Choose one or more gamemodes - Arrow keys to navigate, Space to select, Enter to continue",
				Options: gamemodesList,
			},
			Validate: survey.Required,
		})
	}

	if len(filterscriptsList) > 0 {
		questions = append(questions, &survey.Question{
			Name: "Filterscripts",
			Prompt: &survey.MultiSelect{
				Message: "Choose zero or more filterscripts - Arrow keys to navigate, Space to select, Enter to continue",
				Options: filterscriptsList,
			},
		})
	}

	if len(pluginsList) > 0 {
		questions = append(questions, &survey.Question{
			Name: "Plugins",
			Prompt: &survey.MultiSelect{
				Message: "Choose zero or more plugins - Arrow keys to navigate, Space to select, Enter to continue",
				Options: pluginsList,
			},
		})
	}

	answers := struct {
		Format        string
		Hostname      string
		RCONPassword  string
		Port          int
		MaxPlayers    int
		Gamemodes     []string
		Filterscripts []string
		Plugins       []string
	}{}

	err = survey.Ask(questions, &answers)
	if err != nil {
		return err
	}

	if answers.RCONPassword == "" {
		req := garbler.MakeRequirements("aAbB123_-/#'")
		answers.RCONPassword, err = garbler.NewPassword(&req)
		if err != nil {
			panic(err)
		}
	}

	config := types.Runtime{
		WorkingDir:    dir,
		Format:        answers.Format,
		Hostname:      &answers.Hostname,
		RCONPassword:  &answers.RCONPassword,
		Port:          &answers.Port,
		MaxPlayers:    &answers.MaxPlayers,
		Gamemodes:     answers.Gamemodes,
		Filterscripts: answers.Filterscripts,
	}

	for _, pluginName := range answers.Plugins {
		config.Plugins = append(config.Plugins, types.Plugin(pluginName))
	}

	strength := zxcvbn.PasswordStrength(*config.RCONPassword, nil)

	print.Info("Format: ", answers.Format)
	print.Info("Hostname: ", answers.Hostname)
	print.Info("RCONPassword: ", answers.RCONPassword, " complexity score: ", strength.CrackTimeDisplay)
	print.Info("Port: ", answers.Port)
	print.Info("Max Players: ", answers.MaxPlayers)
	print.Info("Gamemodes: ", answers.Gamemodes)
	print.Info("Filterscripts: ", answers.Filterscripts)
	print.Info("Plugins: ", answers.Plugins)

	err = config.ToFile()
	if err != nil {
		return errors.Wrap(err, "failed to generate config")
	}

	return nil
}

func getAmxFiles(dir string) (result []string) {
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		panic(err)
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		if filepath.Ext(file.Name()) == ".amx" {
			result = append(result, strings.TrimSuffix(file.Name(), filepath.Ext(file.Name())))
		}
	}
	return
}

func getPlugins(dir, platform string) (result []string) {
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		panic(err)
	}

	var ext string
	if platform == "windows" {
		ext = ".dll"
	} else if platform == "linux" || platform == "darwin" {
		ext = ".so"
	} else {
		panic(errors.Errorf("unsupported OS %s", platform))
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		if filepath.Ext(file.Name()) == ext {
			result = append(result, strings.TrimSuffix(file.Name(), filepath.Ext(file.Name())))
		}
	}
	return
}
