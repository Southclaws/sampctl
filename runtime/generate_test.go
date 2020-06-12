package runtime

import (
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Southclaws/sampctl/run"
)

func Test_GenerateServerCfg(t *testing.T) {
	type args struct {
		cfg *run.Runtime
	}
	tests := []struct {
		name    string
		args    args
		wantCfg string
		wantErr bool
	}{
		{
			"servercfg-linux",
			args{&run.Runtime{
				Platform:   "linux",
				WorkingDir: "./tests/generate",
				Announce:   &[]bool{true}[0],
				Hostname:   &[]string{"Test"}[0],
				MaxPlayers: &[]int{32}[0],
				Port:       &[]int{8080}[0],
				RCON:       &[]bool{true}[0],
				Language:   &[]string{"English"}[0],
				Gamemodes: []string{
					"rivershell",
					"baserace",
				},
				Filterscripts: []string{
					"admin",
				},
				Plugins: []run.Plugin{
					"mysql",
				},
				RCONPassword: &[]string{"test"}[0],
			}},
			`echo loading server.cfg generated by sampctl - do not edit this file by hand.
gamemode0 rivershell
gamemode1 baserace
filterscripts admin
plugins mysql.so
rcon_password test
port 8080
hostname Test
maxplayers 32
language English
mapname San Andreas
weburl www.sa-mp.com
gamemodetext Unknown
announce 1
lanmode 0
query 1
rcon 1
logqueries 0
sleep 5
maxnpc 0
stream_rate 1000
stream_distance 200.000000
onfoot_rate 30
incar_rate 30
weapon_rate 30
chatlogging 1
timestamp 1
messageholelimit 3000
messageslimit 500
ackslimit 3000
playertimeout 10000
minconnectiontime 0
lagcompmode 1
connseedtime 300000
db_logging 0
db_log_queries 0
conncookies 1
cookielogging 0
output 1
`,
			false,
		},
		{
			"servercfg-windows",
			args{&run.Runtime{
				Platform:   "windows",
				WorkingDir: "./tests/generate",
				Announce:   &[]bool{true}[0],
				Hostname:   &[]string{"Test"}[0],
				MaxPlayers: &[]int{32}[0],
				Port:       &[]int{8080}[0],
				RCON:       &[]bool{true}[0],
				Language:   &[]string{"English"}[0],
				Gamemodes: []string{
					"rivershell",
					"baserace",
				},
				Filterscripts: []string{
					"admin",
				},
				Plugins: []run.Plugin{
					"mysql",
				},
				RCONPassword: &[]string{"test"}[0],
			}},
			`echo loading server.cfg generated by sampctl - do not edit this file by hand.
gamemode0 rivershell
gamemode1 baserace
filterscripts admin
plugins mysql
rcon_password test
port 8080
hostname Test
maxplayers 32
language English
mapname San Andreas
weburl www.sa-mp.com
gamemodetext Unknown
announce 1
lanmode 0
query 1
rcon 1
logqueries 0
sleep 5
maxnpc 0
stream_rate 1000
stream_distance 200.000000
onfoot_rate 30
incar_rate 30
weapon_rate 30
chatlogging 1
timestamp 1
messageholelimit 3000
messageslimit 500
ackslimit 3000
playertimeout 10000
minconnectiontime 0
lagcompmode 1
connseedtime 300000
db_logging 0
db_log_queries 0
conncookies 1
cookielogging 0
output 1
`,
			false,
		},
		{
			"servercfg-extra",
			args{&run.Runtime{
				Platform:   "windows",
				WorkingDir: "./tests/generate",
				Announce:   &[]bool{true}[0],
				Hostname:   &[]string{"Test"}[0],
				MaxPlayers: &[]int{32}[0],
				Port:       &[]int{8080}[0],
				RCON:       &[]bool{true}[0],
				Language:   &[]string{"English"}[0],
				Gamemodes: []string{
					"rivershell",
					"baserace",
				},
				Filterscripts: []string{
					"admin",
				},
				Plugins: []run.Plugin{
					"mysql",
				},
				RCONPassword: &[]string{"test"}[0],
				Extra: map[string]string{
					"discord_token":  "abc",
					"something_else": "100",
				},
			}},
			`echo loading server.cfg generated by sampctl - do not edit this file by hand.
gamemode0 rivershell
gamemode1 baserace
filterscripts admin
plugins mysql
rcon_password test
port 8080
hostname Test
maxplayers 32
language English
mapname San Andreas
weburl www.sa-mp.com
gamemodetext Unknown
announce 1
lanmode 0
query 1
rcon 1
logqueries 0
sleep 5
maxnpc 0
stream_rate 1000
stream_distance 200.000000
onfoot_rate 30
incar_rate 30
weapon_rate 30
chatlogging 1
timestamp 1
messageholelimit 3000
messageslimit 500
ackslimit 3000
playertimeout 10000
minconnectiontime 0
lagcompmode 1
connseedtime 300000
db_logging 0
db_log_queries 0
conncookies 1
cookielogging 0
output 1
discord_token abc
something_else 100
`,
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := GenerateServerCfg(tt.args.cfg); (err != nil) != tt.wantErr {
				t.Errorf("Config.GenerateServerCfg() error = %v, wantErr %v", err, tt.wantErr)
			}

			raw, _ := ioutil.ReadFile("./tests/generate/server.cfg")
			gotCfg := string(raw)

			assert.Equal(t, tt.wantCfg, gotCfg)
		})
	}
}
