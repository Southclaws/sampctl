package runtime

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"

	"github.com/Southclaws/sampctl/util"
	"github.com/ghodss/yaml"
	"github.com/pkg/errors"
)

// GenerateJSON simply marshals the data to a samp.json file in dir
func (cfg Config) GenerateJSON(dir string) (err error) {
	path := filepath.Join(dir, "samp.json")

	if util.Exists(path) {
		if err := os.Remove(path); err != nil {
			panic(err)
		}
	}

	fh, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0755)
	if err != nil {
		return
	}
	defer func() {
		err := fh.Close()
		if err != nil {
			panic(err)
		}
	}()

	contents, err := json.MarshalIndent(cfg, "", "\t")
	if err != nil {
		return
	}

	_, err = fh.Write(contents)
	return
}

// GenerateYAML simply marshals the data to a samp.yaml file in dir
func (cfg Config) GenerateYAML(dir string) (err error) {
	path := filepath.Join(dir, "samp.yaml")

	if util.Exists(path) {
		if err := os.Remove(path); err != nil {
			panic(err)
		}
	}

	fh, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0755)
	if err != nil {
		return
	}
	defer func() {
		err := fh.Close()
		if err != nil {
			panic(err)
		}
	}()

	contents, err := yaml.Marshal(cfg)
	if err != nil {
		return
	}

	_, err = fh.Write(contents)
	return
}

// GenerateServerCfg creates a settings file in the SA:MP "server.cfg" format at the specified location
func (cfg *Config) GenerateServerCfg(dir string) (err error) {
	file, err := os.Create(filepath.Join(dir, "server.cfg"))
	if err != nil {
		return
	}
	defer func() {
		err := file.Close()
		if err != nil {
			panic(err)
		}
	}()

	// make some minor changes to the cfg before using it
	adjustForOS(dir, cfg)
	cfg.Echo = &echoMessage

	v := reflect.ValueOf(*cfg)
	t := reflect.TypeOf(*cfg)

	for i := 0; i < v.NumField(); i++ {
		fieldval := v.Field(i)
		stype := t.Field(i)

		required := stype.Tag.Get("required") == "1"
		nodefault := stype.Tag.Get("default") == ""
		if !required && nodefault && fieldval.IsNil() {
			continue
		}

		name := strings.Split(stype.Tag.Get("json"), ",")[0]
		real := stype.Tag.Get("cfg") // in case the json version differs from the cfg key
		if real != "" {
			name = real
		}
		defaultValue := stype.Tag.Get("default")
		numbered := stype.Tag.Get("numbered") != ""

		line := ""

		switch stype.Type.String() {
		case "*string":
			line, err = fromString(name, fieldval, required, defaultValue)
		case "[]string":
			line, err = fromSlice(name, fieldval, required, defaultValue, numbered)
		case "*bool":
			line, err = fromBool(name, fieldval, required, defaultValue)
		case "*int":
			line, err = fromInt(name, fieldval, required, defaultValue)
		case "*float32":
			line, err = fromFloat(name, fieldval, required, defaultValue)
		default:
			err = errors.Errorf("unknown kind '%s'", stype.Type.String())
		}
		if err != nil {
			return errors.Wrapf(err, "failed to unpack settings object %s", name)
		}

		_, err := file.WriteString(line)
		if err != nil {
			return errors.Wrap(err, "failed to write setting to server.cfg")
		}
	}

	return
}

// adjustForOS quickly does some tweaks depending on the OS such as .so plugin extension on linux
func adjustForOS(dir string, cfg *Config) {
	if runtime.GOOS == "linux" {
		if len(cfg.Plugins) > 0 {
			actualPlugins := getPlugins(filepath.Join(dir, "plugins"))

			for i, declared := range cfg.Plugins {
				ext := filepath.Ext(string(declared))
				if ext != "" {
					declared = Plugin(strings.TrimSuffix(string(declared), ext))
				}
				for _, actual := range actualPlugins {
					// if the declared plugin matches the found plugin case-insensitively but does match
					// case sensitively...
					if strings.EqualFold(string(declared), actual) && string(declared) != actual {
						// update the array index to use the actual filename
						declared = Plugin(actual)
						break
					}
				}
				cfg.Plugins[i] = declared + ".so"
			}
		}
	}
}

func fromString(name string, obj reflect.Value, required bool, defaultValue string) (result string, err error) {
	var value string

	if obj.IsNil() {
		if required {
			return "", errors.Errorf("field %s is required", name)
		}
		value = defaultValue
	} else {
		value = obj.Elem().String()
	}

	return fmt.Sprintf("%s %s\n", name, value), nil
}

func fromSlice(name string, obj reflect.Value, required bool, defaultValue string, numbered bool) (result string, err error) {
	if obj.IsNil() {
		if required {
			return "", errors.Errorf("field %s is required", name)
		}
		return
	}

	len := obj.Len()

	if numbered {
		for i := 0; i < len; i++ {
			result += fmt.Sprintf("%s%d %s\n", name, i, obj.Index(i).String())
		}
	} else {
		result = name
		for i := 0; i < len; i++ {
			result += fmt.Sprintf(" %s", obj.Index(i).String())
		}
		result += "\n"
	}
	return
}

func fromBool(name string, obj reflect.Value, required bool, defaultValue string) (result string, err error) {
	var value int
	if obj.IsNil() {
		if required {
			return "", errors.Errorf("field %s is required", name)
		}
		value, err = strconv.Atoi(defaultValue)
		if err != nil {
			panic(errors.Wrapf(err, "default bool value %s failed to convert", defaultValue))
		}
	} else {
		if obj.Elem().Bool() {
			value = 1
		} else {
			value = 0
		}
	}

	return fmt.Sprintf("%s %d\n", name, value), nil
}

func fromInt(name string, obj reflect.Value, required bool, defaultValue string) (result string, err error) {
	var value int64

	if obj.IsNil() {
		if required {
			return "", errors.Errorf("field %s is required", name)
		}
		tmp, err := strconv.Atoi(defaultValue)
		if err != nil {
			panic(errors.Wrapf(err, "default int value %s failed to convert", defaultValue))
		}
		value = int64(tmp)
	} else {
		value = obj.Elem().Int()
	}

	return fmt.Sprintf("%s %d\n", name, value), nil
}

func fromFloat(name string, obj reflect.Value, required bool, defaultValue string) (result string, err error) {
	var value float64

	if obj.IsNil() {
		if required {
			return "", errors.Errorf("field %s is required", name)
		}
		value, err = strconv.ParseFloat(defaultValue, 32)
		if err != nil {
			panic(errors.Wrapf(err, "default int value %s failed to convert", defaultValue))
		}
	} else {
		value = obj.Elem().Float()
	}

	return fmt.Sprintf("%s %f\n", name, value), nil
}
