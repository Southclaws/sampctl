package resource

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResourceValidate(t *testing.T) {
	t.Parallel()

	err := (Resource{}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing name")

	err = (Resource{Name: "asset.zip"}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing platform")

	require.NoError(t, (Resource{Name: "asset.zip", Platform: "linux"}).Validate())
}

func TestResourcePath(t *testing.T) {
	t.Parallel()

	res := Resource{Name: "asset.zip"}
	path := res.Path("repo")
	assert.Equal(t, filepath.Dir(path), ".resources")
	assert.Contains(t, filepath.Base(path), "repo-")
}
