package runtime

import (
	"bufio"
	"context"
	"fmt"
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
	sampctltypes "github.com/Southclaws/sampctl/types"
)

// RunContainer does what Run does but inside a Linux container
func RunContainer(cfg sampctltypes.Runtime, cacheDir string) (err error) {
	cli, err := client.NewEnvClient()
	if err != nil {
		return
	}

	args := strslice.StrSlice{"sampctl", "server", "run"}
	for i, arg := range os.Args {
		// trim first 3 args and container specific flags
		if arg == "--container" || arg == "--mountCache" || i < 3 {
			continue
		}
		args = append(args, arg)
	}

	port := fmt.Sprint(*cfg.Port)
	mounts := []mount.Mount{
		{
			Type:   mount.TypeBind,
			Source: cfg.WorkingDir,
			Target: "/samp",
		},
	}

	if cfg.Container.MountCache {
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
	}

	containerName := fmt.Sprintf("sampctl-%d", time.Now().Unix())

	var cnt container.ContainerCreateCreatedBody
	cnt, err = cli.ContainerCreate(
		context.Background(),
		containerConfig,
		hostConfig,
		netConfig,
		containerName)
	if err != nil {
		if client.IsErrNotFound(err) {
			print.Info("Pulling image:", ref)
			pullReader, err := cli.ImagePull(context.Background(), ref, types.ImagePullOptions{})
			if err != nil {
				return errors.Wrap(err, "failed to pull image")
			}
			defer pullReader.Close()
			_, err = ioutil.ReadAll(pullReader)
			if err != nil {
				return errors.Wrap(err, "failed to read pull output")
			}

			cnt, err = cli.ContainerCreate(
				context.Background(),
				containerConfig,
				hostConfig,
				netConfig,
				containerName)
			if err != nil {
				return errors.Wrap(err, "failed to create container")
			}
		} else {
			return errors.Wrap(err, "failed to create container")
		}
	}

	print.Info("Starting container...")
	err = cli.ContainerStart(context.Background(), cnt.ID, types.ContainerStartOptions{})
	if err != nil {
		return errors.Wrap(err, "failed to start container")
	}

	go func() {
		reader, err := cli.ContainerLogs(context.Background(), cnt.ID, types.ContainerLogsOptions{
			ShowStdout: true,
			ShowStderr: true,
			Follow:     true,
			Timestamps: false,
		})
		if err != nil {
			panic(err)
		}
		defer func() {
			if err := reader.Close(); err != nil {
				panic(err)
			}
		}()

		scanner := bufio.NewScanner(reader)
		for scanner.Scan() {
			fmt.Println(scanner.Text())
		}
	}()

	finished := make(chan struct{})

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigs
		err = cli.ContainerKill(context.Background(), cnt.ID, "SIGINT")
		print.Info("server killed:", sig, err)
		finished <- struct{}{}
	}()

	go func() {
		n, err := cli.ContainerWait(context.Background(), cnt.ID)
		print.Erro("container exited:", n, err)
		finished <- struct{}{}
	}()

	<-finished

	return
}
