package runtime

import (
	"context"
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
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

type fakeContainerWaiter struct {
	response container.ContainerWaitOKBody
	err      error
}

func (f fakeContainerWaiter) ContainerWait(_ context.Context, _ string, _ container.WaitCondition) (<-chan container.ContainerWaitOKBody, <-chan error) {
	responseCh := make(chan container.ContainerWaitOKBody, 1)
	errCh := make(chan error, 1)

	if f.err != nil {
		errCh <- f.err
	} else {
		responseCh <- f.response
	}

	return responseCh, errCh
}

func TestWaitForContainerExitAllowsZeroStatus(t *testing.T) {
	t.Parallel()

	err := waitForContainerExit(context.Background(), fakeContainerWaiter{
		response: container.ContainerWaitOKBody{StatusCode: 0},
	}, "container-id")
	require.NoError(t, err)
}

func TestWaitForContainerExitRejectsNonZeroStatus(t *testing.T) {
	t.Parallel()

	err := waitForContainerExit(context.Background(), fakeContainerWaiter{
		response: container.ContainerWaitOKBody{
			StatusCode: 137,
			Error:      &container.ContainerWaitOKBodyError{Message: "terminated"},
		},
	}, "container-id")
	require.EqualError(t, err, "container exited with status code 137: terminated")
}
