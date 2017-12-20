package types

import (
	"github.com/Southclaws/sampctl/versioning"
)

// Runtime stores the server settings and working directory
type Runtime struct {
	// Only used internally
	WorkingDir string `ignore:"1" json:"-"` // local directory that configuration points to

	Platform string `ignore:"1" json:"-"` // the target platform for the runtime

	// Only used to configure sampctl, not used in server.cfg generation
	Version  *string `json:"version,omitempty"`  // SA:MP server binaries version
	Endpoint *string `json:"endpoint,omitempty"` // download endpoint for server binaries

	// Echo - set automatically
	Echo *string `default:"-"             required:"0" json:"echo,omitempty"`

	// Core properties
	Gamemodes     []string `                        json:"gamemodes,omitempty" cfg:"gamemode" numbered:"1"` //
	Filterscripts []string `                        required:"0" json:"filterscripts,omitempty"`            //
	Plugins       []Plugin `                        required:"0" json:"plugins,omitempty"`                  //
	RCONPassword  *string  `required:"1"            json:"rcon_password,omitempty"`                         // changeme
	Port          *int     `default:"8192"          required:"0" json:"port"`                               // 8192
	Hostname      *string  `default:"SA-MP Server"  required:"0" json:"hostname,omitempty"`                 // SA-MP Server
	MaxPlayers    *int     `default:"50"            required:"0" json:"maxplayers"`                         // 50
	Language      *string  `default:"-"             required:"0" json:"language,omitempty"`                 //
	Mapname       *string  `default:"San Andreas"   required:"0" json:"mapname,omitempty"`                  // San Andreas
	Weburl        *string  `default:"www.sa-mp.com" required:"0" json:"weburl,omitempty"`                   // www.sa-mp.com
	GamemodeText  *string  `default:"Unknown"       required:"0" json:"gamemodetext,omitempty"`             // Unknown

	// Network and technical config
	Bind       *string `                        required:"0" json:"bind,omitempty"`       //
	Password   *string `                        required:"0" json:"password,omitempty"`   //
	Announce   *bool   `default:"1"             required:"0" json:"announce,omitempty"`   // 0
	LANMode    *bool   `default:"0"             required:"0" json:"lanmode,omitempty"`    // 0
	Query      *bool   `default:"1"             required:"0" json:"query,omitempty"`      // 0
	RCON       *bool   `default:"0"             required:"0" json:"rcon,omitempty"`       // 0
	LogQueries *bool   `default:"0"             required:"0" json:"logqueries,omitempty"` // 0
	Sleep      *int    `default:"5"             required:"0" json:"sleep,omitempty"`      // 5
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
	Output            *bool    `default:"1"             required:"0" json:"output,omitempty"`            // 1
}

// Plugin represents either a plugin name or a dependency-string description of where to get it
type Plugin string

// GetRuntimeDefault returns a default config for temporary runtimes
func GetRuntimeDefault() (config *Runtime) {
	return &Runtime{
		RCONPassword: &[]string{"temp"}[0],
		Port:         &[]int{7777}[0],
	}
}

// MergeRuntimeDefault returns a default config with the specified config merged on top
func MergeRuntimeDefault(config *Runtime) (result *Runtime) {
	def := GetRuntimeDefault()
	result = config
	if config.RCONPassword == nil {
		result.RCONPassword = def.RCONPassword
	}
	if config.Port == nil {
		result.Port = def.Port
	}
	return
}

// AsDep attempts to interpret the plugin string as a dependency string
func (plugin Plugin) AsDep() (dep versioning.DependencyMeta, err error) {
	depStr := versioning.DependencyString(plugin)
	return depStr.Explode()
}
