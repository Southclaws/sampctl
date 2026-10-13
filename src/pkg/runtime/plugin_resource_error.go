package runtime

import (
	"fmt"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
)

// PluginResourceNotFoundError identifies a plugin without a matching runtime resource.
type PluginResourceNotFoundError struct {
	Meta     versioning.DependencyMeta
	Platform string
	Version  string
}

// Error describes the plugin and the requested platform and runtime version.
func (e *PluginResourceNotFoundError) Error() string {
	name := "plugin"
	if e.Meta.Repo != "" {
		name = e.Meta.String()
	}
	return fmt.Sprintf("%s has no resource for platform %s and runtime version %s", name, e.Platform, e.Version)
}
