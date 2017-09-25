package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

// Config stores the server settings and working directory
type Config struct {
	// Core properties
	Gamemodes     []string `                        json:"gamemodes,omitempty" cfg:"gamemode" numbered:"1"` //
	Filterscripts []string `                        required:"0" json:"filterscripts,omitempty"`            //
	Plugins       []string `                        required:"0" json:"plugins,omitempty"`                  //
	RCONPassword  *string  `required:"1"            json:"rcon_password,omitempty"`                         // changeme
	Port          *int     `default:"8192"          required:"0" json:"port,omitempty"`                     // 8192
	Hostname      *string  `default:"SA-MP Server"  required:"0" json:"hostname,omitempty"`                 // SA-MP Server
	MaxPlayers    *int     `default:"50"            required:"0" json:"maxplayers,omitempty"`               // 50
	Language      *string  `default:"-"             required:"0" json:"language,omitempty"`                 //
	Mapname       *string  `default:"San Andreas"   required:"0" json:"mapname,omitempty"`                  // San Andreas
	Weburl        *string  `default:"www.sa-mp.com" required:"0" json:"weburl,omitempty"`                   // www.sa-mp.com
	GamemodeText  *string  `default:"Unknown"       required:"0" json:"gamemodetext,omitempty"`             // Unknown

	// Network and technical config
	Bind       *string `                        required:"0" json:"bind,omitempty"`       //
	Password   *string `                        required:"0" json:"password,omitempty"`   //
	Announce   *bool   `default:"0"             required:"0" json:"announce,omitempty"`   // 0
	LANMode    *bool   `default:"0"             required:"0" json:"lanmode,omitempty"`    // 0
	Query      *bool   `default:"0"             required:"0" json:"query,omitempty"`      // 0
	RCON       *bool   `default:"0"             required:"0" json:"rcon,omitempty"`       // 0
	LogQueries *bool   `default:"0"             required:"0" json:"logqueries,omitempty"` // 0
	Sleep      *string `default:"5"             required:"0" json:"sleep,omitempty"`      // 5
	MaxNPC     *int    `default:"0"             required:"0" json:"maxnpc,omitempty"`     // 0

	// Rates and performance
	StreamRate        *int     `default:"1000"          required:"0" json:"stream_rate,omitempty"`       // 1000
	StreamDistance    *float32 `default:"200.0"         required:"0" json:"stream_distance,omitempty"`   // 200.0
	OnFootRate        *int     `default:"30"            required:"0" json:"onfoot_rate,omitempty"`       // 30
	InCarRate         *int     `default:"30"            required:"0" json:"incar_rate,omitempty"`        // 30
	WeaponRate        *int     `default:"30"            required:"0" json:"weapon_rate,omitempty"`       // 30
	ChatLogging       *bool    `default:"1"             required:"0" json:"chatlogging,omitempty"`       // 1
	Timestamp         *bool    `default:"1"             required:"0" json:"timestamp,omitempty"`         // 1
	NoSign            *string  `                        required:"0" json:"nosign,omitempty"`            //
	LogTimeFormat     *string  `default:"[%H:%M:%S]"    required:"0" json:"logtimeformat,omitempty"`     // [%H:%M:%S]
	MessageHoleLimit  *int     `default:"3000"          required:"0" json:"messageholelimit,omitempty"`  // 3000
	MessagesLimit     *int     `default:"500"           required:"0" json:"messageslimit,omitempty"`     // 500
	AcksLimit         *int     `default:"3000"          required:"0" json:"ackslimit,omitempty"`         // 3000
	PlayerTimeout     *int     `default:"10000"         required:"0" json:"playertimeout,omitempty"`     // 10000
	MinConnectionTime *int     `default:"0"             required:"0" json:"minconnectiontime,omitempty"` // 0
	LagCompmode       *int     `default:"1"             required:"0" json:"lagcompmode,omitempty"`       // 1
	ConnseedTime      *int     `default:"300000"        required:"0" json:"connseedtime,omitempty"`      // 300000
	DBLogging         *bool    `default:"0"             required:"0" json:"db_logging,omitempty"`        // 0
	DBLogQueries      *bool    `default:"0"             required:"0" json:"db_log_queries,omitempty"`    // 0
	ConnectCookies    *bool    `default:"1"             required:"0" json:"conncookies,omitempty"`       // 1
	CookieLogging     *bool    `default:"0"             required:"0" json:"cookielogging,omitempty"`     // 1
	Output            *bool    `default:"1"             required:"0" json:"output"`                      // 1
}

// NewConfigFromEnvironment creates a Config from the given environment which includes a directory which
// searched for a `samp.json` file and environment variable versions of the config parameters.
func NewConfigFromEnvironment(dir string) (cfg Config, err error) {
	jsonFile := filepath.Join(dir, "samp.json")
	_, err = os.Stat(jsonFile)
	if os.IsNotExist(err) {
		err = nil
	} else if err != nil {
		err = errors.Wrap(err, "failed to stat samp.json")
		return
	} else {
		var contents []byte
		contents, err = ioutil.ReadFile(jsonFile)
		if err != nil {
			err = errors.Wrap(err, "failed to read samp.json")
			return
		}

		err = json.Unmarshal(contents, &cfg)
		if err != nil {
			err = errors.Wrap(err, "failed to unmarshal samp.json")
			return
		}
	}

	// Environment variables override samp.json
	cfg.LoadEnvironmentVariables()

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

// ValidateWorkspace compares a Config to a directory and checks that all the declared gamemodes,
// filterscripts and plugins are present.
func (cfg Config) ValidateWorkspace(dir string) (errs []error) {
	for _, gamemode := range cfg.Gamemodes {
		fullpath := filepath.Join(dir, "gamemodes", gamemode+".amx")
		if !exists(fullpath) {
			errs = append(errs, errors.Errorf("gamemode '%s' is missing its .amx file from the gamemodes directory", gamemode))
		}
	}
	for _, filterscript := range cfg.Filterscripts {
		fullpath := filepath.Join(dir, "filterscripts", filterscript+".amx")
		if !exists(fullpath) {
			errs = append(errs, errors.Errorf("filterscript '%s' is missing its .amx file from the filterscripts directory", filterscript))
		}
	}
	var ext string
	switch runtime.GOOS {
	case "windows":
		ext = ".dll"
	case "linux":
		ext = ".so"
	default:
		errs = append(errs, errors.New("unsupported platform"))
	}
	for _, plugin := range cfg.Plugins {
		fullpath := filepath.Join(dir, plugin, ext)
		if !exists(fullpath) {
			errs = append(errs, errors.Errorf("plugin '%s' is missing its %s file from the plugins directory", plugin, ext))
		}
	}
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

// GenerateJSON simply marshals the data to a samp.json file in dir
func (cfg Config) GenerateJSON(dir string) (err error) {
	path := filepath.Join(dir, "samp.json")

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
