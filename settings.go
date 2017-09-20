package main

import (
	"fmt"
	"os"
	"reflect"
	"strconv"

	"github.com/pkg/errors"
)

// Server stores the server settings and working directory
type Server struct {
	Gamemode          *[]string `required:"1"            json:"gamemode"`                       //
	RCONPassword      *string   `required:"1"            json:"rcon_password"`                  // changeme
	Announce          *bool     `default:"0"             required:"0" json:"announce"`          // 0
	MaxPlayers        *int      `default:"50"            required:"0" json:"maxplayers"`        // 50
	Port              *int      `default:"8192"          required:"0" json:"port"`              // 8192
	LANMode           *bool     `default:"0"             required:"0" json:"lanmode"`           // 0
	Query             *bool     `default:"0"             required:"0" json:"query"`             // 0
	RCON              *bool     `default:"0"             required:"0" json:"rcon"`              // 0
	LogQueries        *bool     `default:"0"             required:"0" json:"logqueries"`        // 0
	StreamRate        *int      `default:"1000"          required:"0" json:"stream_rate"`       // 1000
	StreamDistance    *float32  `default:"200.0"         required:"0" json:"stream_distance"`   // 200.0
	Sleep             *string   `default:"5"             required:"0" json:"sleep"`             // 5
	MaxNPC            *int      `default:"0"             required:"0" json:"maxnpc"`            // 0
	OnFootRate        *int      `default:"30"            required:"0" json:"onfoot_rate"`       // 30
	InCarRate         *int      `default:"30"            required:"0" json:"incar_rate"`        // 30
	WeaponRate        *int      `default:"30"            required:"0" json:"weapon_rate"`       // 30
	ChatLogging       *bool     `default:"1"             required:"0" json:"chatlogging"`       // 1
	Timestamp         *bool     `default:"1"             required:"0" json:"timestamp"`         // 1
	Bind              *string   `default:""              required:"0" json:"bind"`              //
	Password          *string   `default:""              required:"0" json:"password"`          //
	Hostname          *string   `default:"SA-MP Server"  required:"0" json:"hostname"`          // SA-MP Server
	Language          *string   `default:""              required:"0" json:"language"`          //
	Mapname           *string   `default:"San Andreas"   required:"0" json:"mapname"`           // San Andreas
	Weburl            *string   `default:"www.sa-mp.com" required:"0" json:"weburl"`            // www.sa-mp.com
	Gravity           *float32  `default:"0.008"         required:"0" json:"gravity"`           // 0.008
	Weather           *int      `default:"10"            required:"0" json:"weather"`           // 10
	GamemodeText      *string   `default:"Unknown"       required:"0" json:"gamemodetext"`      // Unknown
	Filterscripts     *string   `default:""              required:"0" json:"filterscripts"`     //
	Plugins           *string   `default:""              required:"0" json:"plugins"`           //
	NoSign            *string   `default:""              required:"0" json:"nosign"`            //
	LogTimeFormat     *string   `default:"[%H:%M:%S]"    required:"0" json:"logtimeformat"`     // [%H:%M:%S]
	MessageHoleLimit  *int      `default:"3000"          required:"0" json:"messageholelimit"`  // 3000
	MessagesLimit     *int      `default:"500"           required:"0" json:"messageslimit"`     // 500
	AcksLimit         *int      `default:"3000"          required:"0" json:"ackslimit"`         // 3000
	PlayerTimeout     *int      `default:"10000"         required:"0" json:"playertimeout"`     // 10000
	MinConnectionTime *int      `default:"0"             required:"0" json:"minconnectiontime"` // 0
	Myriad            *int      `default:"50"            required:"0" json:"myriad"`            // 50
	LagCompmode       *int      `default:"1"             required:"0" json:"lagcompmode"`       // 1
	ConnseedTime      *int      `default:"300000"        required:"0" json:"connseedtime"`      // 300000
	DBLogging         *bool     `default:"0"             required:"0" json:"db_logging"`        // 0
	DBLogQueries      *bool     `default:"0"             required:"0" json:"db_log_queries"`    // 0
	ConnectCookies    *bool     `default:"1"             required:"0" json:"conncookies"`       // 1
	CookieLogging     *bool     `default:"0"             required:"0" json:"cookielogging"`     // 1
}

// LoadFromEnv fills a Server with environment variable values
func (server *Server) LoadFromEnv() error {
	// convert json tags to uppercase
	// scan for env vars
	// error on missing if required tag set
	return nil
}

// LoadFromJSON loads settings from JSON
func (server *Server) LoadFromJSON(data []byte) error {
	// error on missing if required tag set
	return nil
}

// Generate creates a settings file in the SA:MP "server.cfg" format at the specified location
func (server *Server) Generate(path string) (err error) {
	file, err := os.Create(path)
	if err != nil {
		return
	}
	defer func() {
		err := file.Close()
		if err != nil {
			panic(err)
		}
	}()

	v := reflect.ValueOf(*server)
	t := reflect.TypeOf(*server)

	for i := 0; i < v.NumField(); i++ {
		fieldval := v.Field(i)
		stype := t.Field(i)

		name := stype.Tag.Get("json")
		required := stype.Tag.Get("required") == "1"
		defaultValue := stype.Tag.Get("default")

		line := ""

		switch stype.Type.String() {
		case "*string":
			line, err = fromString(name, fieldval, required, defaultValue)
		case "*[]string":
			line, err = fromSlice(name, fieldval, required, defaultValue)
		case "*bool":
			line, err = fromBool(name, fieldval, required, defaultValue)
		case "*int":
			line, err = fromInt(name, fieldval, required, defaultValue)
		case "*float32":
			line, err = fromFloat(name, fieldval, required, defaultValue)
		default:
			err = errors.Errorf("unknown kind %v", stype.Type)
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
		} else {
			value = defaultValue
		}
	} else {
		value = obj.Elem().String()
	}

	return fmt.Sprintf("%s %s\n", name, value), nil
}

func fromSlice(name string, obj reflect.Value, required bool, defaultValue string) (result string, err error) {
	elem := obj.Elem()
	len := elem.Len()
	for i := 0; i < len; i++ {
		result += fmt.Sprintf("%s%d %s\n", name, i, elem.Index(i).String())
	}
	return
}

func fromBool(name string, obj reflect.Value, required bool, defaultValue string) (result string, err error) {
	var value int
	if obj.IsNil() {
		if required {
			return "", errors.Errorf("field %s is required", name)
		} else {
			value, err = strconv.Atoi(defaultValue)
			if err != nil {
				panic(errors.Wrapf(err, "default bool value %s failed to convert", defaultValue))
			}
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
		} else {
			tmp, err := strconv.Atoi(defaultValue)
			if err != nil {
				panic(errors.Wrapf(err, "default int value %s failed to convert", defaultValue))
			}
			value = int64(tmp)
		}
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
		} else {
			value, err = strconv.ParseFloat(defaultValue, 32)
			if err != nil {
				panic(errors.Wrapf(err, "default int value %s failed to convert", defaultValue))
			}
		}
	} else {
		value = obj.Elem().Float()
	}

	return fmt.Sprintf("%s %f\n", name, value), nil
}
