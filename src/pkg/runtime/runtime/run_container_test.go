package runtime

import (
	"context"
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/stretchr/testify/require"
)

type fakeContainerRemover struct {
	called      bool
	containerID string
	options     types.ContainerRemoveOptions
}

func (f *fakeContainerRemover) ContainerRemove(_ context.Context, containerID string, options types.ContainerRemoveOptions) error {
	f.called = true
	f.containerID = containerID
	f.options = options
	return nil
}

func TestRemoveContainerUsesForce(t *testing.T) {
	t.Parallel()

	cli := &fakeContainerRemover{}
	err := removeContainer(context.Background(), cli, "container-id")
	require.NoError(t, err)
	require.True(t, cli.called)
	require.Equal(t, "container-id", cli.containerID)
	require.True(t, cli.options.Force)
}

func TestRemoveContainerSkipsEmptyID(t *testing.T) {
	t.Parallel()

	cli := &fakeContainerRemover{}
	err := removeContainer(context.Background(), cli, "")
	require.NoError(t, err)
	require.False(t, cli.called)
}
