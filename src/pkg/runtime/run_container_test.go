package runtime

import (
	"context"
	"errors"
	"testing"
	"time"

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

type fakeContainerStopper struct {
	killCalled   bool
	killSignal   string
	killErr      error
	waitResponse container.ContainerWaitOKBody
	waitErr      error
	waitDelay    time.Duration
}

func (f *fakeContainerStopper) ContainerKill(_ context.Context, _ string, signal string) error {
	f.killCalled = true
	f.killSignal = signal
	return f.killErr
}

func (f *fakeContainerStopper) ContainerWait(ctx context.Context, _ string, _ container.WaitCondition) (<-chan container.ContainerWaitOKBody, <-chan error) {
	responseCh := make(chan container.ContainerWaitOKBody, 1)
	errCh := make(chan error, 1)

	go func() {
		if f.waitDelay > 0 {
			select {
			case <-time.After(f.waitDelay):
			case <-ctx.Done():
				return
			}
		}

		if f.waitErr != nil {
			errCh <- f.waitErr
			return
		}

		responseCh <- f.waitResponse
	}()

	return responseCh, errCh
}

func TestStopContainerSendsSIGINTAndWaits(t *testing.T) {
	t.Parallel()

	cli := &fakeContainerStopper{waitResponse: container.ContainerWaitOKBody{StatusCode: 0}}
	err := stopContainer(context.Background(), cli, "container-id")
	require.NoError(t, err)
	require.True(t, cli.killCalled)
	require.Equal(t, "SIGINT", cli.killSignal)
}

func TestStopContainerReturnsWaitError(t *testing.T) {
	t.Parallel()

	cli := &fakeContainerStopper{waitErr: errors.New("wait failed")}
	err := stopContainer(context.Background(), cli, "container-id")
	require.EqualError(t, err, "wait failed")
	require.True(t, cli.killCalled)
}

func TestStopContainerReturnsContextError(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	cli := &fakeContainerStopper{waitDelay: time.Second}
	err := stopContainer(ctx, cli, "container-id")
	require.ErrorIs(t, err, context.Canceled)
	require.True(t, cli.killCalled)
}
