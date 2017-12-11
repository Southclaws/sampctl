package runtime

import (
	"github.com/Southclaws/sampctl/versioning"
)

// Plugin represents either a plugin name or a dependency-string description of where to get it
type Plugin string

// AsDep attempts to interpret the plugin string as a dependency string
func (plugin Plugin) AsDep() (dep versioning.DependencyMeta, err error) {
	depStr := versioning.DependencyString(plugin)
	return depStr.Explode()
}
