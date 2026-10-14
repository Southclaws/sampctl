package versioning

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSchemeStringPreservesSubpath(t *testing.T) {
	t.Parallel()

	for _, scheme := range []string{"plugin", "component", "includes", "filterscript"} {
		for _, constraint := range []string{"", ":latest", "@main", "#1111111111111111111111111111111111111111"} {
			reference := DependencyString(scheme + "://fixture/shared/subdirectory/includes" + constraint)
			meta, err := reference.ExplodeWithOverrides(nil)
			require.NoError(t, err)
			require.Equal(t, string(reference), meta.String())
		}
	}
}
