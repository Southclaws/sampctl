package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/types"
)

// GenerateServerCfg creates a settings file in the SA:MP "server.cfg" format at the specified location
// nolint:gocyclo
func GenerateServerCfg(cfg *types.Runtime) (err error) {
	file, err := os.Create(filepath.Join(cfg.WorkingDir, "server.cfg"))
	if err != nil {
		return
	}
	defer func() {
		errClose := file.Close()
		if errClose != nil {
			panic(errClose)
		}
	}()

	// make some minor changes to the cfg before using it
	adjustForOS(cfg.WorkingDir, cfg.Platform, cfg)
	cfg.Echo = &echoMessage

	v := reflect.ValueOf(*cfg)
	t := reflect.TypeOf(*cfg)

	for i := 0; i < v.NumField(); i++ {
		fieldval := v.Field(i)
		stype := t.Field(i)

		ignore := stype.Tag.Get("ignore") != ""
		if ignore {
			continue
		}

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
			line, err = fromSlice(name, fieldval, required, numbered)
		case "[]types.Plugin":
			line, err = fromSlice(name, fieldval, required, numbered)
		case "*bool":
			line, err = fromBool(name, fieldval, required, defaultValue)
		case "*int":
			line, err = fromInt(name, fieldval, required, defaultValue)
		case "*float32":
			line, err = fromFloat(name, fieldval, required, defaultValue)
		case "map[string]string":
			line, err = fromMap(name, fieldval, required)
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

	return nil
}

// adjustForOS quickly does some tweaks depending on the OS such as .so plugin extension on linux
func adjustForOS(dir, os string, cfg *types.Runtime) {
	if os == "linux" || os == "darwin" {
		if len(cfg.Plugins) > 0 {
			actualPlugins := getPlugins(filepath.Join(dir, "plugins"), cfg.Platform)

			for i, declared := range cfg.Plugins {
				ext := filepath.Ext(string(declared))
				if ext != "" {
					declared = types.Plugin(strings.TrimSuffix(string(declared), ext))
				}
				for _, actual := range actualPlugins {
					// if the declared plugin matches the found plugin case-insensitively but does match
					// case sensitively...
					if strings.EqualFold(string(declared), actual) && string(declared) != actual {
						// update the array index to use the actual filename
						declared = types.Plugin(actual)
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

func fromSlice(name string, obj reflect.Value, required bool, num bool) (result string, err error) {
	if obj.IsNil() {
		if required {
			return "", errors.Errorf("field %s is required", name)
		}
		return
	}

	len := obj.Len()

	if num {
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
	var value bool
	if obj.IsNil() {
		if required {
			return "", errors.Errorf("field %s is required", name)
		}
		value, err = strconv.ParseBool(defaultValue)
		if err != nil {
			panic(errors.Wrapf(err, "default bool value %s failed to convert", defaultValue))
		}
	} else {
		if obj.Elem().Bool() {
			value = true
		} else {
			value = false
		}
	}
	asInt := 0
	if value {
		asInt = 1
	}

	return fmt.Sprintf("%s %d\n", name, asInt), nil
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

func fromMap(name string, obj reflect.Value, required bool) (result string, err error) {
	if obj.IsNil() {
		if required {
			return "", errors.Errorf("field %s is required", name)
		}
		return
	}

	lines := []string{}
	for _, key := range obj.MapKeys() {
		lines = append(lines, fmt.Sprintf("%s %s", key.String(), obj.MapIndex(key).String()))
	}
	sort.Strings(lines)
	result = strings.Join(lines, "\n") + "\n"

	return
}
