package run

// RuntimeDefaults holds loader defaults used when writing runtime settings.
type RuntimeDefaults struct {
	Port         int
	RCONPassword string
	Hostname     string
	MaxPlayers   int
	Language     string
}

// GetRuntimeDefaultValues returns the runtime loader defaults for fields that
// we serialise conditionally.
func GetRuntimeDefaultValues() RuntimeDefaults {
	return RuntimeDefaults{
		Port:         8192,
		RCONPassword: "",
		Hostname:     "SA-MP Server",
		MaxPlayers:   50,
		Language:     "-",
	}
}
