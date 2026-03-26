package config

import "os/user"

var lookupCurrentUsername = func() string {
	u, err := user.Current()
	if err != nil {
		return ""
	}

	return u.Username
}

func defaultConfig(username string) Config {
	hideVersionUpdateMessage := false

	return Config{
		DefaultUser:              username,
		HideVersionUpdateMessage: &hideVersionUpdateMessage,
	}
}

func normalizeConfig(cfg *Config) {
	if cfg == nil {
		return
	}
	if cfg.HideVersionUpdateMessage == nil {
		hideVersionUpdateMessage := false
		cfg.HideVersionUpdateMessage = &hideVersionUpdateMessage
	}
}
