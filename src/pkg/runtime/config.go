package runtime

import (
	"fmt"
	"io"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	run "github.com/Southclaws/sampctl/src/pkg/runtime/config"
)

type configGenerator interface {
	generate(cfg *run.Runtime) error
	configFilename() string
}

func newConfigGenerator(cfg *run.Runtime) configGenerator {
	if cfg.IsOpenMP() {
		return newOpenMPConfig(cfg.WorkingDir)
	}
	return newSAMPConfig(cfg.WorkingDir)
}

// GenerateConfig generates the runtime configuration file for cfg's effective runtime.
func GenerateConfig(cfg *run.Runtime) error {
	return newConfigGenerator(cfg).generate(cfg)
}

func closeConfigResource(errp *error, closer io.Closer, message string) {
	if closeErr := closer.Close(); closeErr != nil {
		wrapped := errors.Wrap(closeErr, message)
		if *errp == nil {
			*errp = wrapped
			return
		}
		print.Warn(wrapped)
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

	length := obj.Len()

	if num {
		for i := 0; i < length; i++ {
			result += fmt.Sprintf("%s%d %s\n", name, i, obj.Index(i).String())
		}
	} else {
		result = name
		for i := 0; i < length; i++ {
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
			return "", errors.Wrapf(err, "invalid default bool value %q for %s", defaultValue, name)
		}
	} else {
		value = obj.Elem().Bool()
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
			return "", errors.Wrapf(err, "invalid default int value %q for %s", defaultValue, name)
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
			return "", errors.Wrapf(err, "invalid default float value %q for %s", defaultValue, name)
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
