package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/containers/podman/v5/pkg/bindings"
	"github.com/containers/podman/v5/pkg/bindings/containers"
	"github.com/containers/podman/v5/pkg/bindings/images"
	"github.com/containers/podman/v5/pkg/specgen"
)

func stopAndRemoveAllContainer(conn context.Context) {
	// List and delete images with name obdb-*
	containerSummary, err := containers.List(conn, new(containers.ListOptions).WithAll(true))
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	fmt.Println("Deleting images with name obdb-*", containerSummary)

	for _, container := range containerSummary {
		for _, repoTag := range container.Names {
			if strings.HasPrefix(repoTag, "obdb-") {
				// stop first
				err := containers.Stop(conn, container.ID, nil)
				if err != nil {
					fmt.Printf("Error stopping image %s: %v\n", repoTag, err)
				} else {
					fmt.Printf("Stopped image: %s\n", repoTag)
				}

				_, err = containers.Remove(conn, container.ID, nil)
				if err != nil {
					fmt.Printf("Error deleting image %s: %v\n", repoTag, err)
				} else {
					fmt.Printf("Deleted image: %s\n", repoTag)
				}
			}
		}
	}
}

func main() {
	conn, err := bindings.NewConnection(context.Background(), "unix:///run/podman/podman.sock")
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	stopAndRemoveAllContainer(conn)
	defer stopAndRemoveAllContainer(conn)

	// Pull and run hello-world container
	_, err = images.Pull(conn, "hello-world", nil)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	s := specgen.NewSpecGenerator("hello-world", false)
	s.Name = "obdb-hello-world-container"
	createResponse, err := containers.CreateWithSpec(conn, s, nil)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	fmt.Println("Container created.")
	if err := containers.Start(conn, createResponse.ID, nil); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	fmt.Println("Container started.")

	stdoutChan := make(chan string)
	stderrChan := make(chan string)
	go func() {
		for log := range stdoutChan {
			fmt.Print(log)
		}
	}()
	go func() {
		for log := range stderrChan {
			fmt.Print(log)
		}
	}()

	logsOptions := new(containers.LogOptions).WithStdout(true).WithStderr(true)
	err = containers.Logs(conn, createResponse.ID, logsOptions, stdoutChan, stderrChan)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

}
