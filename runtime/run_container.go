package runtime

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/strslice"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/google/uuid"

	sampctltypes "github.com/Southclaws/sampctl/types"
)

// RunContainer does what Run does but inside a Linux container
func RunContainer(cfg sampctltypes.Runtime, cacheDir string) (err error) {
	cli, err := client.NewEnvClient()
	if err != nil {
		panic(err)
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

	cnt, err := cli.ContainerCreate(
		context.Background(),
		&container.Config{
			Image:        "southclaws/sampctl:" + cfg.AppVersion,
			Entrypoint:   args,
			Tty:          true,
			AttachStdout: true,
			AttachStderr: true,
		},
		&container.HostConfig{
			Mounts: mounts,
			PortBindings: nat.PortMap{
				nat.Port(port): []nat.PortBinding{
					{HostIP: "0.0.0.0", HostPort: port},
				},
			},
			SecurityOpt: []string{"seccomp=unconfined"},
			Privileged:  true,
		},
		&network.NetworkingConfig{},
		"sampctl-"+uuid.New().String())
	if err != nil {
		panic(err)
	}

	fmt.Println("Starting container...")
	err = cli.ContainerStart(context.Background(), cnt.ID, types.ContainerStartOptions{})
	if err != nil {
		panic(err)
	}

	fmt.Println("Warnings:", cnt.Warnings)
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
		fmt.Println("server killed:", sig, err)
		finished <- struct{}{}
	}()

	go func() {
		n, err := cli.ContainerWait(context.Background(), cnt.ID)
		fmt.Println("container exited:", n, err)
		finished <- struct{}{}
	}()

	<-finished

	return
}
