package runtime

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"os/signal"
	"syscall"
	"time"

	"github.com/containerd/errdefs"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	run "github.com/Southclaws/sampctl/src/pkg/runtime/config"
)

// RunContainer does what Run does but inside a Linux container
// nolint:gocyclo
func RunContainer(
	ctx context.Context,
	cfg run.Runtime,
	options RunOptions,
) (err error) {
	cli, err := client.New(client.FromEnv)
	if err != nil {
		return
	}
	defer func() {
		if errClose := cli.Close(); errClose != nil {
			if err == nil {
				err = fmt.Errorf("failed to close docker client: %w", errClose)
				return
			}
			print.Warn("failed to close docker client:", errClose)
		}
	}()

	options = options.withDefaults()
	args := append([]string{"sampctl", "run"}, options.Args...)

	port := fmt.Sprint(*cfg.Port)
	containerPort, err := network.ParsePort(port + "/udp")
	if err != nil {
		return fmt.Errorf("invalid container port: %w", err)
	}

	projectDir := options.ProjectDir
	if projectDir == "" {
		projectDir = cfg.WorkingDir
	}

	print.Verb("mounting project directory", projectDir, "at /samp in the container")
	mounts := []mount.Mount{
		{
			Type:   mount.TypeBind,
			Source: projectDir,
			Target: "/samp",
		},
	}

	if cfg.Container.MountCache {
		print.Verb("mounting cache at", options.CacheDir, "into container at /root/.config/sampctl")
		mounts = append(mounts, mount.Mount{
			Type:   mount.TypeBind,
			Source: options.CacheDir,
			Target: "/root/.config/sampctl",
		})
	}

	hostConfig := &container.HostConfig{
		Mounts: mounts,
		PortBindings: network.PortMap{
			containerPort: []network.PortBinding{
				{HostIP: netip.IPv4Unspecified(), HostPort: port},
			},
		},
		SecurityOpt: []string{"seccomp=unconfined"},
		Privileged:  true,
	}

	netConfig := &network.NetworkingConfig{}

	ref := "ghcr.io/southclaws/sampctl:" + cfg.AppVersion
	containerConfig := &container.Config{
		Image:        ref,
		Entrypoint:   args,
		Env:          []string{"XDG_CONFIG_HOME=/root/.config"},
		Tty:          true,
		AttachStdout: true,
		AttachStderr: true,
		AttachStdin:  true,
	}

	containerName := fmt.Sprintf("sampctl-%d", time.Now().Unix())

	ctxPrepare, cancel := context.WithTimeout(ctx, time.Minute*10)
	defer cancel()

	createOptions := client.ContainerCreateOptions{
		Config:           containerConfig,
		HostConfig:       hostConfig,
		NetworkingConfig: netConfig,
		Name:             containerName,
	}
	var cnt client.ContainerCreateResult
	cnt, err = cli.ContainerCreate(ctxPrepare, createOptions)
	if err != nil {
		if errors.Is(err, errdefs.ErrNotFound) {
			print.Info("Pulling image:", ref)
			pullReader, errInner := cli.ImagePull(ctxPrepare, ref, client.ImagePullOptions{})
			if errInner != nil {
				return fmt.Errorf("failed to pull image: %w", errInner)
			}
			defer func() {
				errDefer := pullReader.Close()
				if errDefer != nil {
					print.Erro(errDefer)
				}
			}()

			_, errInner = io.ReadAll(pullReader)
			if errInner != nil {
				return fmt.Errorf("failed to read pull output: %w", errInner)
			}

			cnt, errInner = cli.ContainerCreate(ctxPrepare, createOptions)
			if errInner != nil {
				return fmt.Errorf("failed to create container: %w", errInner)
			}
		} else {
			return fmt.Errorf("failed to create container: %w", err)
		}
	}
	defer func() {
		removeErr := removeContainer(context.Background(), cli, cnt.ID)
		if removeErr == nil {
			return
		}

		if err == nil {
			err = fmt.Errorf("failed to remove container: %w", removeErr)
			return
		}

		print.Warn("failed to remove container:", removeErr)
	}()

	print.Info("Starting container")
	_, err = cli.ContainerStart(ctx, cnt.ID, client.ContainerStartOptions{})
	if err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	runCtx, stopSignals := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stopSignals()

	// Get logs and wait for exit

	reader, err := cli.ContainerLogs(runCtx, cnt.ID, client.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
		Timestamps: false,
	})
	if err != nil {
		return
	}
	defer func() {
		if errClose := reader.Close(); errClose != nil {
			if err == nil {
				err = fmt.Errorf("failed to close container log stream: %w", errClose)
				return
			}
			print.Warn("failed to close container log stream:", errClose)
		}
	}()

	scanner := bufio.NewScanner(reader)
	writeFailed := false
	for scanner.Scan() {
		_, err = fmt.Fprintln(options.Output, scanner.Text())
		if err != nil {
			writeFailed = true
			break
		}
	}

	if scanErr := scanner.Err(); scanErr != nil && runCtx.Err() == nil {
		return fmt.Errorf("failed to read container logs: %w", scanErr)
	}

	if writeFailed {
		stopErr := stopContainer(context.Background(), cli, cnt.ID)
		if stopErr != nil {
			print.Warn("failed to stop container after output error:", stopErr)
		}
		return fmt.Errorf("failed to write container output: %w", err)
	}

	if runCtx.Err() != nil {
		stopErr := stopContainer(context.Background(), cli, cnt.ID)
		if stopErr != nil {
			return fmt.Errorf("failed to stop container after cancellation: %w", stopErr)
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("container stopped after receiving a signal")
	}

	if waitErr := waitForContainerExit(runCtx, cli, cnt.ID); waitErr != nil {
		return fmt.Errorf("container execution failed: %w", waitErr)
	}

	return nil
}

type containerStopper interface {
	ContainerKill(ctx context.Context, containerID string, options client.ContainerKillOptions) (client.ContainerKillResult, error)
	ContainerWait(ctx context.Context, containerID string, options client.ContainerWaitOptions) client.ContainerWaitResult
}

type containerRemover interface {
	ContainerRemove(ctx context.Context, containerID string, options client.ContainerRemoveOptions) (client.ContainerRemoveResult, error)
}

type containerWaiter interface {
	ContainerWait(ctx context.Context, containerID string, options client.ContainerWaitOptions) client.ContainerWaitResult
}

func stopContainer(ctx context.Context, cli containerStopper, containerID string) error {
	stopCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if _, err := cli.ContainerKill(stopCtx, containerID, client.ContainerKillOptions{Signal: "SIGINT"}); err != nil {
		print.Verb("Failed to kill container:", err)
	}

	wait := cli.ContainerWait(stopCtx, containerID, client.ContainerWaitOptions{Condition: container.WaitConditionNotRunning})
	select {
	case <-wait.Result:
		return nil
	case err := <-wait.Error:
		if err == nil {
			return nil
		}
		return err
	case <-stopCtx.Done():
		return stopCtx.Err()
	}
}

func removeContainer(ctx context.Context, cli containerRemover, containerID string) error {
	if containerID == "" {
		return nil
	}

	removeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	_, err := cli.ContainerRemove(removeCtx, containerID, client.ContainerRemoveOptions{Force: true})
	return err
}

func waitForContainerExit(ctx context.Context, cli containerWaiter, containerID string) error {
	wait := cli.ContainerWait(ctx, containerID, client.ContainerWaitOptions{Condition: container.WaitConditionNotRunning})

	select {
	case waitResp := <-wait.Result:
		if waitResp.Error != nil && waitResp.Error.Message != "" {
			return fmt.Errorf("container exited with status code %d: %s", waitResp.StatusCode, waitResp.Error.Message)
		}

		if waitResp.StatusCode != 0 {
			return fmt.Errorf("container exited with status code %d", waitResp.StatusCode)
		}

		return nil
	case err := <-wait.Error:
		if err == nil {
			return nil
		}
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}
