package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

// Config stores the server settings and working directory
type Config struct {
	Gamemodes         []string `                        json:"gamemodes" cfg:"gamemode" numbered:"1"` //
	Gamemode          *string  `                        json:"gamemode" cfg:"gamemode0"`              //
	RCONPassword      *string  `required:"1"            json:"rcon_password"`                         // changeme
	Announce          *bool    `default:"0"             required:"0" json:"announce"`                 // 0
	MaxPlayers        *int     `default:"50"            required:"0" json:"maxplayers"`               // 50
	Port              *int     `default:"8192"          required:"0" json:"port"`                     // 8192
	LANMode           *bool    `default:"0"             required:"0" json:"lanmode"`                  // 0
	Query             *bool    `default:"0"             required:"0" json:"query"`                    // 0
	RCON              *bool    `default:"0"             required:"0" json:"rcon"`                     // 0
	LogQueries        *bool    `default:"0"             required:"0" json:"logqueries"`               // 0
	StreamRate        *int     `default:"1000"          required:"0" json:"stream_rate"`              // 1000
	StreamDistance    *float32 `default:"200.0"         required:"0" json:"stream_distance"`          // 200.0
	Sleep             *string  `default:"5"             required:"0" json:"sleep"`                    // 5
	MaxNPC            *int     `default:"0"             required:"0" json:"maxnpc"`                   // 0
	OnFootRate        *int     `default:"30"            required:"0" json:"onfoot_rate"`              // 30
	InCarRate         *int     `default:"30"            required:"0" json:"incar_rate"`               // 30
	WeaponRate        *int     `default:"30"            required:"0" json:"weapon_rate"`              // 30
	ChatLogging       *bool    `default:"1"             required:"0" json:"chatlogging"`              // 1
	Timestamp         *bool    `default:"1"             required:"0" json:"timestamp"`                // 1
	Bind              *string  `                        required:"0" json:"bind"`                     //
	Password          *string  `                        required:"0" json:"password"`                 //
	Hostname          *string  `default:"SA-MP Server"  required:"0" json:"hostname"`                 // SA-MP Server
	Language          *string  `default:"-"             required:"0" json:"language"`                 //
	Mapname           *string  `default:"San Andreas"   required:"0" json:"mapname"`                  // San Andreas
	Weburl            *string  `default:"www.sa-mp.com" required:"0" json:"weburl"`                   // www.sa-mp.com
	GamemodeText      *string  `default:"Unknown"       required:"0" json:"gamemodetext"`             // Unknown
	Filterscripts     []string `                        required:"0" json:"filterscripts"`            //
	Plugins           []string `                        required:"0" json:"plugins"`                  //
	NoSign            *string  `                        required:"0" json:"nosign"`                   //
	LogTimeFormat     *string  `default:"[%H:%M:%S]"    required:"0" json:"logtimeformat"`            // [%H:%M:%S]
	MessageHoleLimit  *int     `default:"3000"          required:"0" json:"messageholelimit"`         // 3000
	MessagesLimit     *int     `default:"500"           required:"0" json:"messageslimit"`            // 500
	AcksLimit         *int     `default:"3000"          required:"0" json:"ackslimit"`                // 3000
	PlayerTimeout     *int     `default:"10000"         required:"0" json:"playertimeout"`            // 10000
	MinConnectionTime *int     `default:"0"             required:"0" json:"minconnectiontime"`        // 0
	LagCompmode       *int     `default:"1"             required:"0" json:"lagcompmode"`              // 1
	ConnseedTime      *int     `default:"300000"        required:"0" json:"connseedtime"`             // 300000
	DBLogging         *bool    `default:"0"             required:"0" json:"db_logging"`               // 0
	DBLogQueries      *bool    `default:"0"             required:"0" json:"db_log_queries"`           // 0
	ConnectCookies    *bool    `default:"1"             required:"0" json:"conncookies"`              // 1
	CookieLogging     *bool    `default:"0"             required:"0" json:"cookielogging"`            // 1
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
			err = errors.Wrap(err, "failed to stat samp.json")
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

		name := "SAMP_" + strings.ToUpper(t.Field(i).Tag.Get("json"))

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

		case "map[int]string":
			// todo: allow gamemode setting via env vars
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

		name := stype.Tag.Get("json")
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
