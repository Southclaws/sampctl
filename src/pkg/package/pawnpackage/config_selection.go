package pawnpackage

// selectConfig chooses either the first config (if name empty) or the config whose
// name matches the provided selector. Returns false when no configs exist or when
// a specific name cannot be found.
func selectConfig[T any](
	name string,
	configs []*T,
	nameFn func(*T) string,
) (*T, bool) {
	if len(configs) == 0 {
		return nil, false
	}

	if name == "" {
		return configs[0], configs[0] != nil
	}

	for _, cfg := range configs {
		if cfg != nil && nameFn(cfg) == name {
			return cfg, true
		}
	}

	return nil, false
}
