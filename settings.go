package main

// Server stores the server settings and working directory
type Server struct {
	WorkingDir        string `json:"working_dir"`
	Announce          string `default:"0" required:"" json:"announce"`                   //  0
	MaxPlayers        string `default:"50" required:"" json:"maxplayers"`                //  50
	Port              string `default:"8192" required:"" json:"port"`                    //  8192
	LANMode           string `default:"0" required:"" json:"lanmode"`                    //  0
	Query             string `default:"65793?" required:"" json:"query"`                 //  65793?
	RCON              string `default:"65793?" required:"" json:"rcon"`                  //  65793?
	LogQueries        string `default:"0" required:"" json:"logqueries"`                 //  0
	StreamRate        string `default:"1000" required:"" json:"stream_rate"`             //  1000
	StreamDistance    string `default:"200.0" required:"" json:"stream_distance"`        //  200.0
	Sleep             string `default:"5" required:"" json:"sleep"`                      //  5
	MaxNPC            string `default:"0" required:"" json:"maxnpc"`                     //  0
	OnFootRate        string `default:"30" required:"" json:"onfoot_rate"`               //  30
	InCarRate         string `default:"30" required:"" json:"incar_rate"`                //  30
	WeaponRate        string `default:"30" required:"" json:"weapon_rate"`               //  30
	ChatLogging       string `default:"1" required:"" json:"chatlogging"`                //  1
	Timestamp         string `default:"1" required:"" json:"timestamp"`                  //  1
	Bind              string `default:"<empty string>" required:"" json:"bind"`          //  <empty string>
	Password          string `default:"<empty string>" required:"" json:"password"`      //  <empty string>
	Hostname          string `default:"SA-MP Server" required:"" json:"hostname"`        //  SA-MP Server
	Language          string `default:"<empty string>" required:"" json:"language"`      //  <empty string>
	Mapname           string `default:"San Andreas" required:"" json:"mapname"`          //  San Andreas
	Weburl            string `default:"www.sa-mp.com" required:"" json:"weburl"`         //  www.sa-mp.com
	RCONPassword      string `default:"changeme" required:"" json:"rcon_password"`       //  changeme
	Gravity           string `default:"0.008" required:"" json:"gravity"`                //  0.008
	Weather           string `default:"10" required:"" json:"weather"`                   //  10
	GamemodeText      string `default:"Unknown" required:"" json:"gamemodetext"`         //  Unknown
	Filterscripts     string `default:"<empty string>" required:"" json:"filterscripts"` //  <empty string>
	Plugins           string `default:"<empty string>" required:"" json:"plugins"`       //  <empty string>
	NoSign            string `default:"<empty string>" required:"" json:"nosign"`        //  <empty string>
	LogTimeFormat     string `default:"[%H:%M:%S]" required:"" json:"logtimeformat"`     //  [%H:%M:%S]
	MessageHoleLimit  string `default:"3000" required:"" json:"messageholelimit"`        //  3000
	MessagesLimit     string `default:"500" required:"" json:"messageslimit"`            //  500
	AcksLimit         string `default:"3000" required:"" json:"ackslimit"`               //  3000
	PlayerTimeout     string `default:"10000" required:"" json:"playertimeout"`          //  10000
	MinConnectionTime string `default:"0" required:"" json:"minconnectiontime"`          //  0
	Myriad            string `default:"50?" required:"" json:"myriad"`                   //  50?
	LagCompmode       string `default:"1" required:"" json:"lagcompmode"`                //  1
	ConnseedTime      string `default:"300000" required:"" json:"connseedtime"`          //  300000
	DBLogging         string `default:"0" required:"" json:"db_logging"`                 //  0
	DBLogQueries      string `default:"0" required:"" json:"db_log_queries"`             //  0
	ConnectCookies    string `default:"1" required:"" json:"conncookies"`                //  1
	CookieLogging     string `default:"1" required:"" json:"cookielogging"`              //  1
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
