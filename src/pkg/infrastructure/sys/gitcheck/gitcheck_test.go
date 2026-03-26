package gitcheck

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsInstalled(t *testing.T) {
	t.Parallel()
	assert.True(t, IsInstalled())
}

func TestRequireInstalled(t *testing.T) {
	t.Parallel()
	assert.NoError(t, RequireInstalled())
}
