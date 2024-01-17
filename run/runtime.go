package run

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"

	"github.com/Southclaws/sampctl/print"
	"github.com/Southclaws/sampctl/util"
	"github.com/Southclaws/sampctl/versioning"
)

// Runtime stores the server settings and working directory
// nolint:lll
type Runtime struct {
	// Only used internally
	WorkingDir string                      `ignore:"1" json:"-" yaml:"-"` // local directory that configuration points to
	Platform   string                      `ignore:"1" json:"-" yaml:"-"` // the target platform for the runtime
	Container  *ContainerConfig            `ignore:"1" json:"-" yaml:"-"` // configuration for container runtime
	AppVersion string                      `ignore:"1" json:"-" yaml:"-"` // app version for container runtime
	PluginDeps []versioning.DependencyMeta `ignore:"1" json:"-" yaml:"-"` // an internal list of remote plugins to download
	Format     string                      `ignore:"1" json:"-" yaml:"-"` // format stores the original format of the package definition file, either `json` or `yaml`

	// Only used to configure sampctl, not used in server.cfg generation
	Name     string  `ignore:"1" json:"name,omitempty"     yaml:"name,omitempty"`                    // configuration name
	Version  string  `ignore:"1" json:"version,omitempty"  yaml:"version,omitempty"`                 // runtime version
	Mode     RunMode `ignore:"1" json:"mode,omitempty"     yaml:"mode,omitempty"`                    // the runtime mode
	RootLink bool    `ignore:"1" default:"true" json:"rootLink,omitempty" yaml:"rootLink,omitempty"` // toggles creating a symlink to the root directory. https://github.com/Southclaws/sampctl/issues/248

	Echo *string `ignore:"1" json:"echo,omitempty" yaml:"echo,omitempty"`

	// Core properties
	Gamemodes     []string `cfg:"gamemode" numbered:"1"          json:"gamemodes,omitempty"     yaml:"gamemodes,omitempty"`     //
	Filterscripts []string `                        required:"0" json:"filterscripts,omitempty" yaml:"filterscripts,omitempty"` //
	Plugins       []Plugin `                        required:"0" json:"plugins,omitempty"       yaml:"plugins,omitempty"`       //
	RCONPassword  *string  `                        required:"1" json:"rcon_password,omitempty" yaml:"rcon_password,omitempty"` // changeme
	Port          *int     `default:"8192"          required:"0" json:"port,omitempty"          yaml:"port,omitempty"`          // 8192
	Hostname      *string  `default:"SA-MP Server"  required:"0" json:"hostname,omitempty"      yaml:"hostname,omitempty"`      // SA-MP Server
	MaxPlayers    *int     `default:"50"            required:"0" json:"maxplayers,omitempty"    yaml:"maxplayers,omitempty"`    // 50
	Language      *string  `default:"-"             required:"0" json:"language,omitempty"      yaml:"language,omitempty"`      //
	Mapname       *string  `default:"San Andreas"   required:"0" json:"mapname,omitempty"       yaml:"mapname,omitempty"`       // San Andreas
	Weburl        *string  `default:"www.sa-mp.com" required:"0" json:"weburl,omitempty"        yaml:"weburl,omitempty"`        // www.sa-mp.com
	GamemodeText  *string  `default:"Unknown"       required:"0" json:"gamemodetext,omitempty"  yaml:"gamemodetext,omitempty"`  // Unknown

	// Network and technical config
	Bind       *string `                required:"0" json:"bind,omitempty"       yaml:"bind,omitempty"`       //
	Password   *string `                required:"0" json:"password,omitempty"   yaml:"password,omitempty"`   //
	Announce   *bool   `default:"true"  required:"0" json:"announce,omitempty"   yaml:"announce,omitempty"`   // 0
	LANMode    *bool   `default:"false" required:"0" json:"lanmode,omitempty"    yaml:"lanmode,omitempty"`    // 0
	Query      *bool   `default:"true"  required:"0" json:"query,omitempty"      yaml:"query,omitempty"`      // 0
	RCON       *bool   `default:"false" required:"0" json:"rcon,omitempty"       yaml:"rcon,omitempty"`       // 0
	LogQueries *bool   `default:"false" required:"0" json:"logqueries,omitempty" yaml:"logqueries,omitempty"` // 0
	Sleep      *int    `default:"5"     required:"0" json:"sleep,omitempty"      yaml:"sleep,omitempty"`      // 5
	MaxNPC     *int    `default:"0"     required:"0" json:"maxnpc,omitempty"     yaml:"maxnpc,omitempty"`     // 0

	// Rates and performance
	StreamRate        *int     `default:"1000"         required:"0" json:"stream_rate,omitempty"       yaml:"stream_rate,omitempty"`       // 1000
	StreamDistance    *float32 `default:"200.0"        required:"0" json:"stream_distance,omitempty"   yaml:"stream_distance,omitempty"`   // 200.0
	OnFootRate        *int     `default:"30"           required:"0" json:"onfoot_rate,omitempty"       yaml:"onfoot_rate,omitempty"`       // 30
	InCarRate         *int     `default:"30"           required:"0" json:"incar_rate,omitempty"        yaml:"incar_rate,omitempty"`        // 30
	WeaponRate        *int     `default:"30"           required:"0" json:"weapon_rate,omitempty"       yaml:"weapon_rate,omitempty"`       // 30
	ChatLogging       *bool    `default:"true"         required:"0" json:"chatlogging,omitempty"       yaml:"chatlogging,omitempty"`       // 1
	Timestamp         *bool    `default:"true"         required:"0" json:"timestamp,omitempty"         yaml:"timestamp,omitempty"`         // 1
	NoSign            *string  `                       required:"0" json:"nosign,omitempty"            yaml:"nosign,omitempty"`            //
	LogTimeFormat     *string  `                       required:"0" json:"logtimeformat,omitempty"     yaml:"logtimeformat,omitempty"`     // [%H:%M:%S]
	MessageHoleLimit  *int     `default:"3000"         required:"0" json:"messageholelimit,omitempty"  yaml:"messageholelimit,omitempty"`  // 3000
	MessagesLimit     *int     `default:"500"          required:"0" json:"messageslimit,omitempty"     yaml:"messageslimit,omitempty"`     // 500
	AcksLimit         *int     `default:"3000"         required:"0" json:"ackslimit,omitempty"         yaml:"ackslimit,omitempty"`         // 3000
	PlayerTimeout     *int     `default:"10000"        required:"0" json:"playertimeout,omitempty"     yaml:"playertimeout,omitempty"`     // 10000
	MinConnectionTime *int     `default:"0"            required:"0" json:"minconnectiontime,omitempty" yaml:"minconnectiontime,omitempty"` // 0
	LagCompmode       *int     `default:"1"            required:"0" json:"lagcompmode,omitempty"       yaml:"lagcompmode,omitempty"`       // 1
	ConnseedTime      *int     `default:"300000"       required:"0" json:"connseedtime,omitempty"      yaml:"connseedtime,omitempty"`      // 300000
	DBLogging         *bool    `default:"false"        required:"0" json:"db_logging,omitempty"        yaml:"db_logging,omitempty"`        // 0
	DBLogQueries      *bool    `default:"false"        required:"0" json:"db_log_queries,omitempty"    yaml:"db_log_queries,omitempty"`    // 0
	ConnectCookies    *bool    `default:"true"         required:"0" json:"conncookies,omitempty"       yaml:"conncookies,omitempty"`       // 1
	CookieLogging     *bool    `default:"false"        required:"0" json:"cookielogging,omitempty"     yaml:"cookielogging,omitempty"`     // 1
	Output            *bool    `default:"true"         required:"0" json:"output,omitempty"            yaml:"output,omitempty"`            // 1

	// Extra properties for plugins etc
	Extra map[string]string `required:"0" json:"extra,omitempty" yaml:"extra,omitempty"`
}

// ContainerConfig is used if the runtime is specified to run inside a container
type ContainerConfig struct {
	MountCache bool // whether or not to mount the local cache directory inside the container
}

// RunMode represents a method of running the server
type RunMode string

const (
	// Server is the normal runtime mode, it just runs the server as a server
	Server RunMode = "server"
	// MainOnly hides preamble and closes the server after the main() function finishes
	MainOnly RunMode = "main"
	// YTesting hides preamble and closes the server after y_testing output has finished
	YTesting RunMode = "y_testing"
)

// Plugin represents either a plugin name or a dependency-string description of where to get it
type Plugin string

// Validate checks a Runtime for missing fields
func (cfg Runtime) Validate() (err error) {
	if cfg.WorkingDir == "" {
		return errors.New("WorkingDir empty")
	}

	if cfg.Platform == "" {
		return errors.New("Platform empty")
	}

	if cfg.Format == "" {
		return errors.New("Format empty")
	}

	if cfg.Version == "" {
		return errors.New("Version empty")
	}

	if cfg.Mode == "" {
		return errors.New("Mode empty")
	}

	if cfg.Echo == nil {
		cfg.Echo = new(string)
		*cfg.Echo = ""
	}

	return
}

// RuntimeFromDir creates a config from a directory by searching for a JSON or YAML file to
// read settings from. If both exist, the JSON file takes precedence.
func RuntimeFromDir(dir string) (cfg Runtime, err error) {
	jsonFile := filepath.Join(dir, "samp.json")
	if util.Exists(jsonFile) {
		cfg, err = RuntimeFromJSON(jsonFile)
		if err != nil {
			return
		}
		cfg.WorkingDir = dir
		return
	}

	yamlFile := filepath.Join(dir, "samp.yaml")
	if util.Exists(yamlFile) {
		cfg, err = RuntimeFromYAML(yamlFile)
		if err != nil {
			return
		}
		cfg.WorkingDir = dir
		return
	}

	err = errors.New("directory does not contain a samp.json or samp.yaml file")
	return
}

// RuntimeFromJSON creates a config from a JSON file
func RuntimeFromJSON(file string) (cfg Runtime, err error) {
	var contents []byte
	contents, err = ioutil.ReadFile(file)
	if err != nil {
		err = errors.Wrap(err, "failed to read samp.json")
		return
	}

	err = json.Unmarshal(contents, &cfg)
	if err != nil {
		err = errors.Wrap(err, "failed to unmarshal samp.json")
		return
	}
	cfg.Format = "json"

	return
}

// RuntimeFromYAML creates a config from a YAML file
func RuntimeFromYAML(file string) (cfg Runtime, err error) {
	var contents []byte
	contents, err = ioutil.ReadFile(file)
	if err != nil {
		err = errors.Wrap(err, "failed to read samp.json")
		return
	}

	err = yaml.Unmarshal(contents, &cfg)
	if err != nil {
		err = errors.Wrap(err, "failed to unmarshal samp.json")
		return
	}
	cfg.Format = "yaml"

	return
}

// ResolveRemotePlugins separates simple plugin filenames from dependency strings
func (cfg *Runtime) ResolveRemotePlugins() {
	if cfg == nil {
		return
	}

	print.Verb("separating dep plugins from:", cfg.Plugins)

	tmpPlugins := cfg.Plugins
	cfg.Plugins = []Plugin{}

	// separate depstrings from regular plugins
	for _, plugin := range tmpPlugins {
		dep, err := plugin.AsDep()
		if err != nil {
			cfg.Plugins = append(cfg.Plugins, plugin)
		} else {
			cfg.PluginDeps = append(cfg.PluginDeps, dep)
		}
	}
}

// GetRuntimeDefault returns a default config for temporary runtimes
func GetRuntimeDefault() (config *Runtime) {
	return &Runtime{
		Version:      "0.3.7",
		RCONPassword: &[]string{"password"}[0],
		Port:         &[]int{7777}[0],
		Mode:         Server,
	}
}

// ApplyRuntimeDefaults modifies the input runtime config to apply defaults to
// empty fields
func ApplyRuntimeDefaults(rt *Runtime) {
	if rt == nil {
		panic("cannot apply runtime defaults to nil pointer")
	}

	def := GetRuntimeDefault()

	if rt.Version == "" {
		rt.Version = def.Version
	}
	if rt.Platform == "" {
		rt.Platform = runtime.GOOS
	}
	if rt.RCONPassword == nil {
		rt.RCONPassword = def.RCONPassword
	}
	if rt.Port == nil {
		rt.Port = def.Port
	}
	if rt.Mode == "" {
		rt.Mode = def.Mode
	}
}

// AsDep attempts to interpret the plugin string as a dependency string
func (plugin Plugin) AsDep() (dep versioning.DependencyMeta, err error) {
	depStr := versioning.DependencyString(plugin)
	return depStr.Explode()
}

// ToFile creates a JSON or YAML file for a config object, the format depends
// on the `Format` field of the package.
func (cfg Runtime) ToFile() (err error) {
	switch cfg.Format {
	case "json":
		err = cfg.ToJSON()
	case "yaml":
		err = cfg.ToYAML()
	default:
		err = errors.New("package has no format associated with it")
	}
	return
}

// ToJSON simply marshals the data to a samp.json file in dir
func (cfg Runtime) ToJSON() (err error) {
	path := filepath.Join(cfg.WorkingDir, "samp.json")

	if util.Exists(path) {
		if err = os.Remove(path); err != nil {
			panic(err)
		}
	}

	fh, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	defer func() {
		err = fh.Close()
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

// ToYAML simply marshals the data to a samp.yaml file in dir
func (cfg Runtime) ToYAML() (err error) {
	path := filepath.Join(cfg.WorkingDir, "samp.yaml")

	if util.Exists(path) {
		if err = os.Remove(path); err != nil {
			panic(err)
		}
	}

	fh, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	defer func() {
		if err = fh.Close(); err != nil {
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
