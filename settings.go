package main

import (
	"fmt"
	"os"
	"reflect"

	"github.com/pkg/errors"
)

// Server stores the server settings and working directory
type Server struct {
	Announce          string `default:"0"             required:"1" json:"announce"`          // 0
	MaxPlayers        string `default:"50"            required:"1" json:"maxplayers"`        // 50
	Port              string `default:"8192"          required:"1" json:"port"`              // 8192
	LANMode           string `default:"0"             required:"0" json:"lanmode"`           // 0
	Query             string `default:"65793?"        required:"0" json:"query"`             // 65793?
	RCON              string `default:"65793?"        required:"1" json:"rcon"`              // 65793?
	LogQueries        string `default:"0"             required:"0" json:"logqueries"`        // 0
	StreamRate        string `default:"1000"          required:"0" json:"stream_rate"`       // 1000
	StreamDistance    string `default:"200.0"         required:"0" json:"stream_distance"`   // 200.0
	Sleep             string `default:"5"             required:"0" json:"sleep"`             // 5
	MaxNPC            string `default:"0"             required:"0" json:"maxnpc"`            // 0
	OnFootRate        string `default:"30"            required:"0" json:"onfoot_rate"`       // 30
	InCarRate         string `default:"30"            required:"0" json:"incar_rate"`        // 30
	WeaponRate        string `default:"30"            required:"0" json:"weapon_rate"`       // 30
	ChatLogging       string `default:"1"             required:"0" json:"chatlogging"`       // 1
	Timestamp         string `default:"1"             required:"0" json:"timestamp"`         // 1
	Bind              string `default:""              required:"0" json:"bind"`              //
	Password          string `default:""              required:"0" json:"password"`          //
	Hostname          string `default:"SA-MP Server"  required:"1" json:"hostname"`          // SA-MP Server
	Language          string `default:""              required:"1" json:"language"`          //
	Mapname           string `default:"San Andreas"   required:"0" json:"mapname"`           // San Andreas
	Weburl            string `default:"www.sa-mp.com" required:"0" json:"weburl"`            // www.sa-mp.com
	RCONPassword      string `default:"changeme"      required:"1" json:"rcon_password"`     // changeme
	Gravity           string `default:"0.008"         required:"0" json:"gravity"`           // 0.008
	Weather           string `default:"10"            required:"0" json:"weather"`           // 10
	GamemodeText      string `default:"Unknown"       required:"0" json:"gamemodetext"`      // Unknown
	Filterscripts     string `default:""              required:"0" json:"filterscripts"`     //
	Plugins           string `default:""              required:"0" json:"plugins"`           //
	NoSign            string `default:""              required:"0" json:"nosign"`            //
	LogTimeFormat     string `default:"[%H:%M:%S]"    required:"0" json:"logtimeformat"`     // [%H:%M:%S]
	MessageHoleLimit  string `default:"3000"          required:"0" json:"messageholelimit"`  // 3000
	MessagesLimit     string `default:"500"           required:"0" json:"messageslimit"`     // 500
	AcksLimit         string `default:"3000"          required:"0" json:"ackslimit"`         // 3000
	PlayerTimeout     string `default:"10000"         required:"0" json:"playertimeout"`     // 10000
	MinConnectionTime string `default:"0"             required:"0" json:"minconnectiontime"` // 0
	Myriad            string `default:"50?"           required:"0" json:"myriad"`            // 50?
	LagCompmode       string `default:"1"             required:"0" json:"lagcompmode"`       // 1
	ConnseedTime      string `default:"300000"        required:"0" json:"connseedtime"`      // 300000
	DBLogging         string `default:"0"             required:"0" json:"db_logging"`        // 0
	DBLogQueries      string `default:"0"             required:"0" json:"db_log_queries"`    // 0
	ConnectCookies    string `default:"1"             required:"0" json:"conncookies"`       // 1
	CookieLogging     string `default:"1"             required:"0" json:"cookielogging"`     // 1
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
		field := v.Field(i)
		setting := field.String()
		structField := t.Field(i)

		if setting != "" {
			name := structField.Tag.Get("json")
			_, err := file.WriteString(fmt.Sprintf("%s=%s\n", name, setting))
			if err != nil {
				return errors.Wrap(err, "failed to write setting to server.cfg")
			}
		} else {
			if structField.Tag.Get("required") == "1" {
				return errors.Errorf("field %s is required", structField.Name)
			}
		}
	}

	return
}
