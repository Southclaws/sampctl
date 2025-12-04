package run

// RuntimeDefaults holds the default values that configor applies to Runtime fields.
// Keeping these values alongside the Runtime type ensures writers can stay in sync
// with loader behaviour when new fields are introduced.
type RuntimeDefaults struct {
	Port         int
	RCONPassword string
	Hostname     string
	MaxPlayers   int
	Language     string
}

// GetRuntimeDefaultValues returns the configor defaults for runtime fields that
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
