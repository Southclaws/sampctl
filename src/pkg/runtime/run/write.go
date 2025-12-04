package run

// CloneWithoutDefaults creates a copy of Runtime with default values removed so
// configor-applied defaults are not persisted back to disk.
func CloneWithoutDefaults(rt *Runtime) *Runtime {
	if rt == nil {
		return nil
	}

	clean := &Runtime{
		WorkingDir:    rt.WorkingDir,
		Platform:      rt.Platform,
		Container:     rt.Container,
		AppVersion:    rt.AppVersion,
		PluginDeps:    rt.PluginDeps,
		Format:        rt.Format,
		Name:          rt.Name,
		Version:       rt.Version,
		Echo:          rt.Echo,
		Gamemodes:     rt.Gamemodes,
		Filterscripts: rt.Filterscripts,
		Plugins:       rt.Plugins,
	}

	if rt.Mode != "" && rt.Mode != Server {
		clean.Mode = rt.Mode
	}

	if rt.RuntimeType != "" {
		clean.RuntimeType = rt.RuntimeType
	}

	if !rt.RootLink {
		clean.RootLink = rt.RootLink
	}

	defaults := GetRuntimeDefaultValues()

	if rt.RCONPassword != nil && *rt.RCONPassword != defaults.RCONPassword {
		clean.RCONPassword = rt.RCONPassword
	}
	if rt.Port != nil && *rt.Port != defaults.Port {
		clean.Port = rt.Port
	}
	if rt.Hostname != nil && *rt.Hostname != defaults.Hostname {
		clean.Hostname = rt.Hostname
	}
	if rt.MaxPlayers != nil && *rt.MaxPlayers != defaults.MaxPlayers {
		clean.MaxPlayers = rt.MaxPlayers
	}

	if rt.Language != nil && *rt.Language != defaults.Language && *rt.Language != "" {
		clean.Language = rt.Language
	}

	clean.Mapname = rt.Mapname
	clean.Weburl = rt.Weburl
	clean.GamemodeText = rt.GamemodeText
	clean.Bind = rt.Bind
	clean.Password = rt.Password
	clean.Announce = rt.Announce
	clean.LANMode = rt.LANMode
	clean.Query = rt.Query
	clean.RCON = rt.RCON
	clean.LogQueries = rt.LogQueries
	clean.Sleep = rt.Sleep
	clean.MaxNPC = rt.MaxNPC
	clean.StreamRate = rt.StreamRate
	clean.StreamDistance = rt.StreamDistance
	clean.OnFootRate = rt.OnFootRate
	clean.InCarRate = rt.InCarRate
	clean.WeaponRate = rt.WeaponRate
	clean.ChatLogging = rt.ChatLogging
	clean.Timestamp = rt.Timestamp
	clean.NoSign = rt.NoSign
	clean.LogTimeFormat = rt.LogTimeFormat
	clean.MessageHoleLimit = rt.MessageHoleLimit
	clean.MessagesLimit = rt.MessagesLimit
	clean.AcksLimit = rt.AcksLimit
	clean.PlayerTimeout = rt.PlayerTimeout
	clean.MinConnectionTime = rt.MinConnectionTime
	clean.LagCompmode = rt.LagCompmode
	clean.ConnseedTime = rt.ConnseedTime
	clean.DBLogging = rt.DBLogging
	clean.DBLogQueries = rt.DBLogQueries

	return clean
}
