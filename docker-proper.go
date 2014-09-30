package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/samalba/dockerclient"
)

var (
	now          = time.Now()
	addr         = flag.String("a", "unix:///var/run/docker.sock", "address of docker daemon")
	ageContainer = flag.Duration("ca", 4*7*24*time.Hour, "Max container age")
	ageImage     = flag.Duration("ia", 4*7*24*time.Hour, "Max images age")

	dry     = flag.Bool("dry", false, "Dry run; do not actually delete")
	verbose = flag.Bool("v", false, "Be verbose")
)

func debug(fs string, args ...interface{}) {
	if *verbose {
		log.Printf(fs, args...)
	}
}

func main() {
	flag.Parse()
	client, err := dockerclient.NewDockerClient(*addr, nil)
	if err != nil {
		log.Fatal(err)
	}
	if err := cleanup(client); err != nil {
		log.Fatal(err)
	}
}

func cleanup(client *dockerclient.DockerClient) error {
	expiredContainers, err := getExpiredContainers(client)
	if err != nil {
		return err
	}
	log.Printf("Found %d expired containers", len(expiredContainers))
	for _, container := range expiredContainers {
		log.Printf("rm %s (%s)", container.Id, container.Name)
		if !*dry {
			if err := client.RemoveContainer(container.Id); err != nil {
				return fmt.Errorf("Couldn't remove container: %s", err)
			}
		}
	}

	expiredImages, err := getExpiredImages(client)
	if err != nil {
		return err
	}
	log.Printf("Found %d expired images", len(expiredImages))
	for _, image := range expiredImages {
		log.Print("rmi ", image.Id)
		if !*dry {
			if err := client.RemoveImage(image.Id); err != nil {
				if derr, ok := err.(dockerclient.Error); ok {
					if derr.StatusCode == http.StatusConflict {
						continue // ignore images in use
					}
				}
				return fmt.Errorf("Couldn't remove image: %s", err)
			}
		}
	}
	return nil
}

func getExpiredContainers(client *dockerclient.DockerClient) ([]*dockerclient.ContainerInfo, error) {
	containers, err := client.ListContainers(true) // true = all containers
	if err != nil {
		return nil, err
	}
	expiredContainers := []*dockerclient.ContainerInfo{}
	for _, c := range containers {
		debug("< container: %s", c.Id)
		container, err := client.InspectContainer(c.Id)
		if err != nil {
			return nil, fmt.Errorf("Couldn't inspect container %s: %s", c.Id, err)
		}
		if container.State.Running {
			continue
		}
		debug("  + not running")
		if container.State.FinishedAt.After(now.Add(-*ageContainer)) {
			continue
		}
		debug("  + older than %s", *ageContainer)
		expiredContainers = append(expiredContainers, container)
	}
	return expiredContainers, nil
}

// getExpiredImages() returns all image older than ia weeks. They *may* be still in use!
func getExpiredImages(c *dockerclient.DockerClient) ([]*dockerclient.Image, error) {
	images, err := c.ListImages()
	if err != nil {
		return nil, err
	}
	expiredImages := []*dockerclient.Image{}
	for _, image := range images {
		debug("< image: %s", image.Id)
		ctime := time.Unix(image.Created, 0)
		if ctime.After(now.Add(-*ageImage)) {
			continue
		}
		debug("  + older than %s", *ageImage)
		expiredImages = append(expiredImages, image)
	}
	return expiredImages, nil
}
