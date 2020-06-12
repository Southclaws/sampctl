package runtime

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/strslice"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/print"
	"github.com/Southclaws/sampctl/run"
)

// RunContainer does what Run does but inside a Linux container
// nolint:gocyclo
func RunContainer(
	ctx context.Context,
	cfg run.Runtime,
	cacheDir string,
	passArgs bool,
	output io.Writer,
	input io.Reader,
) (err error) {
	cli, err := client.NewEnvClient()
	if err != nil {
		return
	}

	args := strslice.StrSlice{"sampctl", "server", "run"}
	if passArgs {
		for i, arg := range os.Args {
			// trim first 3 args and container specific flags
			if arg == "--container" || arg == "--mountCache" || i < 3 {
				continue
			}
			args = append(args, arg)
		}
	}

	port := fmt.Sprint(*cfg.Port)
	print.Verb("mounting package working directory at", cfg.WorkingDir, "into container at /samp")
	mounts := []mount.Mount{
		{
			Type:   mount.TypeBind,
			Source: cfg.WorkingDir,
			Target: "/samp",
		},
	}

	if cfg.Container.MountCache {
		print.Verb("mounting cache at", cacheDir, "into container at /root/.samp")
		mounts = append(mounts, mount.Mount{
			Type:     mount.TypeBind,
			Source:   cacheDir,
			Target:   "/root/.samp",
			ReadOnly: true,
		})
	}

	hostConfig := &container.HostConfig{
		Mounts: mounts,
		PortBindings: nat.PortMap{
			nat.Port(port): []nat.PortBinding{
				{HostIP: "0.0.0.0", HostPort: port},
			},
		},
		SecurityOpt: []string{"seccomp=unconfined"},
		Privileged:  true,
	}

	netConfig := &network.NetworkingConfig{
		//
	}

	ref := "southclaws/sampctl:" + cfg.AppVersion
	containerConfig := &container.Config{
		Image:        ref,
		Entrypoint:   args,
		Tty:          true,
		AttachStdout: true,
		AttachStderr: true,
		AttachStdin:  true,
	}

	containerName := fmt.Sprintf("sampctl-%d", time.Now().Unix())

	ctxPrepare, cancel := context.WithTimeout(ctx, time.Minute*10)
	defer cancel()

	var cnt container.ContainerCreateCreatedBody
	cnt, err = cli.ContainerCreate(
		ctxPrepare,
		containerConfig,
		hostConfig,
		netConfig,
		containerName)
	if err != nil {
		if client.IsErrNotFound(err) {
			print.Info("Pulling image:", ref)
			pullReader, errInner := cli.ImagePull(ctxPrepare, ref, types.ImagePullOptions{})
			if errInner != nil {
				return errors.Wrap(errInner, "failed to pull image")
			}
			defer func() {
				errDefer := pullReader.Close()
				if errDefer != nil {
					print.Erro(errDefer)
				}
			}()
			_, errInner = ioutil.ReadAll(pullReader)
			if errInner != nil {
				return errors.Wrap(errInner, "failed to read pull output")
			}

			cnt, errInner = cli.ContainerCreate(
				ctxPrepare,
				containerConfig,
				hostConfig,
				netConfig,
				containerName)
			if errInner != nil {
				return errors.Wrap(errInner, "failed to create container")
			}
		} else {
			return errors.Wrap(err, "failed to create container")
		}
	}

	print.Info("Starting container...")
	err = cli.ContainerStart(ctx, cnt.ID, types.ContainerStartOptions{})
	if err != nil {
		return errors.Wrap(err, "failed to start container")
	}

	finished := make(chan error, 1)

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigs
		finished <- errors.Errorf("killed: %s", sig)
	}()

	go func() {
		n, errInner := cli.ContainerWait(context.Background(), cnt.ID)
		if errInner != nil {
			if errInner.Error() == "context deadline exceeded" {
				errInner = nil
			}
		}
		print.Erro("container exited:", n, errInner)
		finished <- errInner
	}()

	// Get logs and wait for exit

	reader, err := cli.ContainerLogs(ctx, cnt.ID, types.ContainerLogsOptions{
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
			panic(errClose)
		}
	}()

	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		_, err = fmt.Fprintln(output, scanner.Text())
		if err != nil {
			break
		}
	}
	if err != nil {
		print.Erro("Failed to write to output:", err)
	}

	err = cli.ContainerKill(ctx, cnt.ID, "SIGINT")
	if err != nil {
		print.Verb("Failed to kill container:", err)
		err = nil
	}

	return err
}
