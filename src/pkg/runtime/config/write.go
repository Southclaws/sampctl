package run

import "encoding/json"

// CloneWithoutDefaults creates a copy of Runtime with default values removed so
// loader-applied defaults are not persisted back to disk.
func CloneWithoutDefaults(rt *Runtime) *Runtime {
	if rt == nil {
		return nil
	}

	clean := *rt

	if clean.Mode == Server {
		clean.Mode = ""
	}

	defaults := GetRuntimeDefaultValues()

	if clean.RCONPassword != nil && *clean.RCONPassword == defaults.RCONPassword {
		clean.RCONPassword = nil
	}
	if clean.Port != nil && *clean.Port == defaults.Port {
		clean.Port = nil
	}
	if clean.Hostname != nil && *clean.Hostname == defaults.Hostname {
		clean.Hostname = nil
	}
	if clean.MaxPlayers != nil && *clean.MaxPlayers == defaults.MaxPlayers {
		clean.MaxPlayers = nil
	}

	if clean.Language != nil && (*clean.Language == defaults.Language || *clean.Language == "") {
		clean.Language = nil
	}

	if runtimeDefinitionIsEmpty(&clean) {
		return nil
	}

	return &clean
}

func runtimeDefinitionIsEmpty(rt *Runtime) bool {
	if rt == nil {
		return true
	}

	data, err := json.Marshal(rt)
	if err != nil {
		return false
	}

	return string(data) == "{}"
}
