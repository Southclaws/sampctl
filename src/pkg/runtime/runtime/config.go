package runtime

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/transform"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	"github.com/Southclaws/sampctl/src/pkg/runtime/run"
)

type ConfigGenerator interface {
	Generate(cfg *run.Runtime) error
	GetConfigFilename() string
}

type SAMPConfig struct {
	workingDir string
}

type OpenMPConfig struct {
	workingDir string
}

type OpenMPConfigData struct {
	Name        string  `json:"name,omitempty"`
	MaxPlayers  int     `json:"max_players,omitempty"`
	MaxBots     int     `json:"max_bots,omitempty"`
	Language    string  `json:"language,omitempty"`
	Password    string  `json:"password,omitempty"`
	Announce    bool    `json:"announce"`
	EnableQuery bool    `json:"enable_query"`
	Website     string  `json:"website,omitempty"`
	Sleep       float64 `json:"sleep,omitempty"`
	UseDynTicks bool    `json:"use_dyn_ticks"`

	Game    *OpenMPGameConfig    `json:"game,omitempty"`
	Network *OpenMPNetworkConfig `json:"network,omitempty"`
	Logging *OpenMPLoggingConfig `json:"logging,omitempty"`
	RCON    *OpenMPRCONConfig    `json:"rcon,omitempty"`
	Pawn    *OpenMPPawnConfig    `json:"pawn,omitempty"`
	Discord *OpenMPDiscordConfig `json:"discord,omitempty"`
	Banners *OpenMPBannersConfig `json:"banners,omitempty"`
	Logo    string               `json:"logo,omitempty"`
	Artwork *OpenMPArtworkConfig `json:"artwork,omitempty"`

	Extra map[string]interface{} `json:"-"`
}

type OpenMPGameConfig struct {
	AllowInteriorWeapons      *bool    `json:"allow_interior_weapons"`
	ChatRadius                *float64 `json:"chat_radius,omitempty"`
	DeathDropAmount           *int     `json:"death_drop_amount,omitempty"`
	Gravity                   *float64 `json:"gravity,omitempty"`
	GroupPlayerObjects        *bool    `json:"group_player_objects,omitempty"`
	LagCompensationMode       *int     `json:"lag_compensation_mode,omitempty"`
	Map                       *string  `json:"map,omitempty"`
	Mode                      *string  `json:"mode,omitempty"`
	NametagDrawRadius         *float64 `json:"nametag_draw_radius,omitempty"`
	PlayerMarkerDrawRadius    *float64 `json:"player_marker_draw_radius,omitempty"`
	PlayerMarkerMode          *int     `json:"player_marker_mode,omitempty"`
	Time                      *int     `json:"time,omitempty"`
	UseAllAnimations          *bool    `json:"use_all_animations,omitempty"`
	UseChatRadius             *bool    `json:"use_chat_radius,omitempty"`
	UseEntryExitMarkers       *bool    `json:"use_entry_exit_markers"`
	UseInstagib               *bool    `json:"use_instagib,omitempty"`
	UseManualEngineAndLights  *bool    `json:"use_manual_engine_and_lights,omitempty"`
	UseNametagLOS             *bool    `json:"use_nametag_los"`
	UseNametags               *bool    `json:"use_nametags"`
	UsePlayerMarkerDrawRadius *bool    `json:"use_player_marker_draw_radius,omitempty"`
	UsePlayerPedAnims         *bool    `json:"use_player_ped_anims,omitempty"`
	UseStuntBonuses           *bool    `json:"use_stunt_bonuses"`
	UseVehicleFriendlyFire    *bool    `json:"use_vehicle_friendly_fire,omitempty"`
	UseZoneNames              *bool    `json:"use_zone_names,omitempty"`
	ValidateAnimations        *bool    `json:"validate_animations"`
	VehicleRespawnTime        *int     `json:"vehicle_respawn_time,omitempty"`
	Weather                   *int     `json:"weather,omitempty"`
}

type OpenMPNetworkConfig struct {
	AcksLimit             *int     `json:"acks_limit,omitempty"`
	AimingSyncRate        *int     `json:"aiming_sync_rate,omitempty"`
	Allow037Clients       *bool    `json:"allow_037_clients"`
	UseOMPEncryption      *bool    `json:"use_omp_encryption,omitempty"`
	Bind                  *string  `json:"bind,omitempty"`
	CookieReseedTime      *int     `json:"cookie_reseed_time,omitempty"`
	GracePeriod           *int     `json:"grace_period,omitempty"`
	HTTPThreads           *int     `json:"http_threads,omitempty"`
	InVehicleSyncRate     *int     `json:"in_vehicle_sync_rate,omitempty"`
	LimitsBanTime         *int     `json:"limits_ban_time,omitempty"`
	MessageHoleLimit      *int     `json:"message_hole_limit,omitempty"`
	MessagesLimit         *int     `json:"messages_limit,omitempty"`
	MinimumConnectionTime *int     `json:"minimum_connection_time,omitempty"`
	MTU                   *int     `json:"mtu,omitempty"`
	Multiplier            *int     `json:"multiplier,omitempty"`
	OnFootSyncRate        *int     `json:"on_foot_sync_rate,omitempty"`
	PlayerMarkerSyncRate  *int     `json:"player_marker_sync_rate,omitempty"`
	PlayerTimeout         *int     `json:"player_timeout,omitempty"`
	Port                  *int     `json:"port,omitempty"`
	PublicAddr            *string  `json:"public_addr,omitempty"`
	StreamRadius          *float64 `json:"stream_radius,omitempty"`
	StreamRate            *int     `json:"stream_rate,omitempty"`
	TimeSyncRate          *int     `json:"time_sync_rate,omitempty"`
	UseLANMode            *bool    `json:"use_lan_mode,omitempty"`
}

type OpenMPLoggingConfig struct {
	Enable                *bool   `json:"enable"`
	File                  *string `json:"file,omitempty"`
	LogChat               *bool   `json:"log_chat"`
	LogConnectionMessages *bool   `json:"log_connection_messages"`
	LogCookies            *bool   `json:"log_cookies,omitempty"`
	LogDeaths             *bool   `json:"log_deaths"`
	LogQueries            *bool   `json:"log_queries,omitempty"`
	LogSQLite             *bool   `json:"log_sqlite,omitempty"`
	LogSQLiteQueries      *bool   `json:"log_sqlite_queries,omitempty"`
	TimestampFormat       *string `json:"timestamp_format,omitempty"`
	UsePrefix             *bool   `json:"use_prefix"`
	UseTimestamp          *bool   `json:"use_timestamp"`
}

type OpenMPRCONConfig struct {
	AllowTeleport *bool   `json:"allow_teleport,omitempty"`
	Enable        *bool   `json:"enable,omitempty"`
	Password      *string `json:"password,omitempty"`
}

type OpenMPPawnConfig struct {
	LegacyPlugins []string `json:"legacy_plugins,omitempty"`
	Components    []string `json:"components,omitempty"`
	MainScripts   []string `json:"main_scripts,omitempty"`
	SideScripts   []string `json:"side_scripts,omitempty"`
}

type OpenMPDiscordConfig struct {
	Invite string `json:"invite,omitempty"`
}

type OpenMPBannersConfig struct {
	Light string `json:"light,omitempty"`
	Dark  string `json:"dark,omitempty"`
}

type OpenMPArtworkConfig struct {
	CDN           *string `json:"cdn,omitempty"`
	Enable        *bool   `json:"enable"`
	ModelsPath    *string `json:"models_path,omitempty"`
	Port          *int    `json:"port,omitempty"`
	WebServerBind *string `json:"web_server_bind,omitempty"`
}

func NewSAMPConfig(workingDir string) *SAMPConfig {
	return &SAMPConfig{workingDir: workingDir}
}

func NewOpenMPConfig(workingDir string) *OpenMPConfig {
	return &OpenMPConfig{workingDir: workingDir}
}

func (s *SAMPConfig) GetConfigFilename() string {
	return "server.cfg"
}

func (o *OpenMPConfig) GetConfigFilename() string {
	return "config.json"
}

// Generate creates a SA-MP server.cfg file
func (s *SAMPConfig) Generate(cfg *run.Runtime) error {
	// Remove any existing config.json file to avoid confusion
	configJSONPath := filepath.Join(s.workingDir, "config.json")
	if _, err := os.Stat(configJSONPath); err == nil {
		print.Verb("Removing existing config.json file (SA-MP uses server.cfg)")
		os.Remove(configJSONPath)
	}

	file, err := os.Create(filepath.Join(s.workingDir, "server.cfg"))
	if err != nil {
		return err
	}
	defer func() {
		errClose := file.Close()
		if errClose != nil {
			panic(errClose)
		}
	}()

	encoder := charmap.Windows1252.NewEncoder()
	writer := transform.NewWriter(file, encoder)

	adjustForOS(s.workingDir, cfg.Platform, cfg)
	_, err = io.WriteString(writer, "echo loading server.cfg generated by sampctl - do not edit this file by hand.\n")
	if err != nil {
		return err
	}

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
		real := stype.Tag.Get("cfg")
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
		case "[]run.Plugin":
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

		_, err := io.WriteString(writer, line)
		if err != nil {
			return errors.Wrap(err, "failed to write setting to server.cfg")
		}
	}

	return nil
}

func (o *OpenMPConfig) Generate(cfg *run.Runtime) error {
	// Remove any existing server.cfg file to avoid confusion
	serverCfgPath := filepath.Join(o.workingDir, "server.cfg")
	if _, err := os.Stat(serverCfgPath); err == nil {
		print.Verb("Removing existing server.cfg file (open.mp uses config.json)")
		os.Remove(serverCfgPath)
	}

	config := &OpenMPConfigData{
		UseDynTicks: true,
		Extra:       make(map[string]interface{}),
	}

	if cfg.Hostname != nil {
		config.Name = *cfg.Hostname
	}
	if cfg.MaxPlayers != nil {
		config.MaxPlayers = *cfg.MaxPlayers
	}
	if cfg.Language != nil && *cfg.Language != "-" {
		config.Language = *cfg.Language
	}
	if cfg.Password != nil {
		config.Password = *cfg.Password
	}
	if cfg.Announce != nil {
		config.Announce = *cfg.Announce
	}
	if cfg.Query != nil {
		config.EnableQuery = *cfg.Query
	}
	if cfg.Weburl != nil && *cfg.Weburl != "open.mp" {
		config.Website = *cfg.Weburl
	}
	if cfg.Sleep != nil {
		config.Sleep = float64(*cfg.Sleep)
	}

	defaultTrue := true
	config.Game = &OpenMPGameConfig{
		AllowInteriorWeapons: &defaultTrue,
		UseEntryExitMarkers:  &defaultTrue,
		UseNametagLOS:        &defaultTrue,
		UseNametags:          &defaultTrue,
		UseStuntBonuses:      &defaultTrue,
		ValidateAnimations:   &defaultTrue,
	}
	config.Network = &OpenMPNetworkConfig{
		Allow037Clients: &defaultTrue,
	}
	config.Logging = &OpenMPLoggingConfig{
		Enable:                &defaultTrue,
		LogChat:               &defaultTrue,
		LogConnectionMessages: &defaultTrue,
		LogDeaths:             &defaultTrue,
		UsePrefix:             &defaultTrue,
		UseTimestamp:          &defaultTrue,
	}
	config.RCON = &OpenMPRCONConfig{}
	config.Pawn = &OpenMPPawnConfig{}

	if cfg.LagCompmode != nil {
		config.Game.LagCompensationMode = cfg.LagCompmode
	}
	if cfg.Mapname != nil && *cfg.Mapname != "San Andreas" {
		config.Game.Map = cfg.Mapname
	}
	if cfg.GamemodeText != nil && *cfg.GamemodeText != "Unknown" {
		config.Game.Mode = cfg.GamemodeText
	}

	if cfg.Port != nil {
		config.Network.Port = cfg.Port
	}
	if cfg.Bind != nil {
		config.Network.Bind = cfg.Bind
	}
	if cfg.OnFootRate != nil {
		config.Network.OnFootSyncRate = cfg.OnFootRate
	}
	if cfg.InCarRate != nil {
		config.Network.InVehicleSyncRate = cfg.InCarRate
	}
	if cfg.WeaponRate != nil {
		config.Network.AimingSyncRate = cfg.WeaponRate
	}
	if cfg.StreamRate != nil {
		config.Network.StreamRate = cfg.StreamRate
	}
	if cfg.StreamDistance != nil {
		distance := float64(*cfg.StreamDistance)
		config.Network.StreamRadius = &distance
	}
	if cfg.MessageHoleLimit != nil {
		config.Network.MessageHoleLimit = cfg.MessageHoleLimit
	}
	if cfg.MessagesLimit != nil {
		config.Network.MessagesLimit = cfg.MessagesLimit
	}
	if cfg.AcksLimit != nil {
		config.Network.AcksLimit = cfg.AcksLimit
	}
	if cfg.PlayerTimeout != nil {
		config.Network.PlayerTimeout = cfg.PlayerTimeout
	}
	if cfg.MinConnectionTime != nil {
		config.Network.MinimumConnectionTime = cfg.MinConnectionTime
	}
	if cfg.ConnseedTime != nil {
		config.Network.CookieReseedTime = cfg.ConnseedTime
	}
	if cfg.LANMode != nil {
		config.Network.UseLANMode = cfg.LANMode
	}

	if cfg.Output != nil {
		config.Logging.Enable = cfg.Output
	}
	if cfg.ChatLogging != nil {
		config.Logging.LogChat = cfg.ChatLogging
	}
	if cfg.LogQueries != nil {
		config.Logging.LogQueries = cfg.LogQueries
	}
	if cfg.CookieLogging != nil {
		config.Logging.LogCookies = cfg.CookieLogging
	}
	if cfg.DBLogging != nil {
		config.Logging.LogSQLite = cfg.DBLogging
	}
	if cfg.DBLogQueries != nil {
		config.Logging.LogSQLiteQueries = cfg.DBLogQueries
	}
	if cfg.Timestamp != nil {
		config.Logging.UseTimestamp = cfg.Timestamp
	}
	if cfg.LogTimeFormat != nil {
		config.Logging.TimestampFormat = cfg.LogTimeFormat
	}

	if cfg.RCON != nil {
		config.RCON.Enable = cfg.RCON
	}
	if cfg.RCONPassword != nil {
		config.RCON.Password = cfg.RCONPassword
	}

	if len(cfg.Plugins) > 0 {
		plugins := make([]string, len(cfg.Plugins))
		for i, plugin := range cfg.Plugins {
			// Remove .so/.dll extension for open.mp
			pluginStr := string(plugin)
			if strings.HasSuffix(pluginStr, ".so") || strings.HasSuffix(pluginStr, ".dll") {
				pluginStr = strings.TrimSuffix(pluginStr, filepath.Ext(pluginStr))
			}
			plugins[i] = pluginStr
		}
		config.Pawn.LegacyPlugins = plugins
	}

	if len(cfg.Components) > 0 {
		components := make([]string, len(cfg.Components))
		for i, component := range cfg.Components {
			componentStr := string(component)
			if strings.HasSuffix(componentStr, ".so") || strings.HasSuffix(componentStr, ".dll") {
				componentStr = strings.TrimSuffix(componentStr, filepath.Ext(componentStr))
			}
			components[i] = componentStr
		}
		config.Pawn.Components = components
	}

	if len(cfg.Gamemodes) > 0 {
		config.Pawn.MainScripts = cfg.Gamemodes
	}

	if len(cfg.Filterscripts) > 0 {
		config.Pawn.SideScripts = cfg.Filterscripts
	}

	if cfg.Extra != nil {
		for key, value := range cfg.Extra {
			config.Extra[key] = value
		}
	}

	jsonData := make(map[string]interface{})

	structuredBytes, err := json.Marshal(config)
	if err != nil {
		return errors.Wrap(err, "failed to marshal structured config")
	}

	err = json.Unmarshal(structuredBytes, &jsonData)
	if err != nil {
		return errors.Wrap(err, "failed to unmarshal structured config")
	}

	for key, value := range config.Extra {
		jsonData[key] = value
	}

	file, err := os.Create(filepath.Join(o.workingDir, "config.json"))
	if err != nil {
		return err
	}
	defer func() {
		errClose := file.Close()
		if errClose != nil {
			panic(errClose)
		}
	}()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	err = encoder.Encode(jsonData)
	if err != nil {
		return errors.Wrap(err, "failed to write config.json")
	}

	// Create server.cfg for legacy plugin compatibility
	err = o.generateLegacyServerCfg(cfg)
	if err != nil {
		return errors.Wrap(err, "failed to generate legacy server.cfg")
	}

	return nil
}

// generateLegacyServerCfg creates a server.cfg file with extras for legacy plugin compatibility
func (o *OpenMPConfig) generateLegacyServerCfg(cfg *run.Runtime) error {
	serverCfgPath := filepath.Join(o.workingDir, "server.cfg")

	// Only create server.cfg if there are extra configuration values
	if len(cfg.Extra) == 0 {
		print.Verb("No extra configuration found, skipping server.cfg generation")
		return nil
	}

	print.Verb("Generating server.cfg for legacy plugin compatibility")

	file, err := os.Create(serverCfgPath)
	if err != nil {
		return errors.Wrap(err, "failed to create server.cfg")
	}
	defer func() {
		errClose := file.Close()
		if errClose != nil {
			panic(errClose)
		}
	}()

	encoder := charmap.Windows1252.NewEncoder()
	writer := transform.NewWriter(file, encoder)

	// Write header comment
	_, err = io.WriteString(writer, "# server.cfg generated by sampctl for legacy plugin compatibility\n")
	if err != nil {
		return errors.Wrap(err, "failed to write header to server.cfg")
	}

	// Write extra configuration values
	// Sort keys for consistent output
	keys := make([]string, 0, len(cfg.Extra))
	for key := range cfg.Extra {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		value := cfg.Extra[key]

		// Write the key-value pair to server.cfg
		line := fmt.Sprintf("%s %s\n", key, value)
		_, err = io.WriteString(writer, line)
		if err != nil {
			return errors.Wrapf(err, "failed to write config line '%s' to server.cfg", key)
		}
	}

	print.Verb("Generated server.cfg with", len(cfg.Extra), "extra configuration values")
	return nil
}

func GetConfigGenerator(cfg *run.Runtime) ConfigGenerator {
	if cfg.IsOpenMP() {
		return NewOpenMPConfig(cfg.WorkingDir)
	}
	return NewSAMPConfig(cfg.WorkingDir)
}

func GenerateConfig(cfg *run.Runtime) error {
	generator := GetConfigGenerator(cfg)
	return generator.Generate(cfg)
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
