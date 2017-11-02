package settings

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"

	"github.com/Southclaws/sampctl/util"
	"github.com/ghodss/yaml"
	"github.com/pkg/errors"
)

// ConfigFromDirectory creates a config from a directory by searching for a JSON or YAML file to
// read settings from. If both exist, the JSON file takes precedence.
func ConfigFromDirectory(dir string) (cfg Config, err error) {
	jsonFile := filepath.Join(dir, "samp.json")
	if util.Exists(jsonFile) {
		cfg, err = ConfigFromJSON(jsonFile)
	} else {
		yamlFile := filepath.Join(dir, "samp.yaml")
		if util.Exists(yamlFile) {
			cfg, err = ConfigFromYAML(yamlFile)
		} else {
			err = errors.New("directory does not contain a samp.json or samp.yaml file")
		}
	}

	return
}

// ConfigFromJSON creates a config from a JSON file
func ConfigFromJSON(file string) (cfg Config, err error) {
	var contents []byte
	contents, err = ioutil.ReadFile(file)
	if err != nil {
		err = errors.Wrap(err, "failed to read samp.json")
		return
	}

	err = json.Unmarshal(contents, &cfg)
	if err != nil {
		err = errors.Wrap(err, "failed to unmarshal samp.json")
		return
	}

	return
}

// ConfigFromYAML creates a config from a YAML file
func ConfigFromYAML(file string) (cfg Config, err error) {
	var contents []byte
	contents, err = ioutil.ReadFile(file)
	if err != nil {
		err = errors.Wrap(err, "failed to read samp.json")
		return
	}

	err = yaml.Unmarshal(contents, &cfg)
	if err != nil {
		err = errors.Wrap(err, "failed to unmarshal samp.json")
		return
	}

	return
}

// LoadEnvironmentVariables loads Config fields from environment variables - the variable names are
// simply the `json` tag names uppercased and prefixed with `SAMP_`
func (cfg *Config) LoadEnvironmentVariables() {
	v := reflect.ValueOf(cfg).Elem()
	t := v.Type()

	for i := 0; i < t.NumField(); i++ {
		fieldval := v.Field(i)
		stype := t.Field(i)

		if !fieldval.CanSet() {
			continue
		}

		name := "SAMP_" + strings.ToUpper(strings.Split(t.Field(i).Tag.Get("json"), ",")[0])

		value, ok := os.LookupEnv(name)
		if !ok {
			continue
		}

		switch stype.Type.String() {
		case "*string":
			if fieldval.IsNil() {
				v := reflect.ValueOf(value)
				fieldval.Set(reflect.New(v.Type()))
			}
			fieldval.Elem().SetString(value)

		case "[]string":
			// todo: allow filterscripts and plugins via env vars
			fmt.Println("cannot set gamemode via environment variables yet")

		case "*bool":
			valueAsBool, err := strconv.ParseBool(value)
			if err != nil {
				fmt.Printf("warning: environment variable '%s' could not interpret value '%s' as boolean: %v\n", stype.Name, value, err)
			}
			if fieldval.IsNil() {
				v := reflect.ValueOf(valueAsBool)
				fieldval.Set(reflect.New(v.Type()))
			}
			fieldval.Elem().SetBool(valueAsBool)

		case "*int":
			valueAsInt, err := strconv.Atoi(value)
			if err != nil {
				fmt.Printf("warning: environment variable '%s' could not interpret value '%s' as integer: %v\n", stype.Name, value, err)
				continue
			}
			if fieldval.IsNil() {
				v := reflect.ValueOf(valueAsInt)
				fieldval.Set(reflect.New(v.Type()))
			}
			fieldval.Elem().SetInt(int64(valueAsInt))

		case "*float32":
			valueAsFloat, err := strconv.ParseFloat(value, 64)
			if err != nil {
				fmt.Printf("warning: environment variable '%s' could not interpret value '%s' as float: %v\n", stype.Name, value, err)
				continue
			}
			if fieldval.IsNil() {
				v := reflect.ValueOf(valueAsFloat)
				fieldval.Set(reflect.New(v.Type()))
			}
			fieldval.Elem().SetFloat(valueAsFloat)
		default:
			panic(fmt.Sprintf("unknown kind '%s'", stype.Type.String()))
		}
	}
}
