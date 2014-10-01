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
	interval     = flag.Duration("r", 0, "Run continously in given interval")

	dry     = flag.Bool("dry", false, "Dry run; do not actually delete")
	unsafe  = flag.Bool("u", false, "Unsafe; Delete container without FinishedAt field set")
	verbose = flag.Bool("v", false, "Be verbose")
)

func debug(fs string, args ...interface{}) {
	if *verbose {
		log.Printf(fs, args...)
	}
}

func main() {
	flag.Parse()
	for {
		client, err := dockerclient.NewDockerClient(*addr, nil)
		if err != nil {
			log.Fatal(err)
		}
		if err := cleanup(client); err != nil {
			log.Fatal(err)
		}
		if *interval == 0 {
			break
		}
		log.Printf("Sleeping for %s", *interval)
		time.Sleep(*interval)
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
	usedVolumeContainers := map[string]bool{}
	oldContainers := []*dockerclient.ContainerInfo{}
	for _, c := range containers {
		debug("< container: %s", c.Id)
		container, err := client.InspectContainer(c.Id)
		if err != nil {
			return nil, fmt.Errorf("Couldn't inspect container %s: %s", c.Id, err)
		}
		if len(container.HostConfig.VolumesFrom) > 0 {
			for _, vc := range container.HostConfig.VolumesFrom {
				usedVolumeContainers[vc] = true
			}
		}
		if container.State.Running {
			continue
		}
		debug("  + not running")

		if container.State.FinishedAt == time.Date(1, 1, 1, 0, 0, 0, 0, time.UTC) && !*unsafe {
			return nil, fmt.Errorf("Container %s has empty FinishedAt field", c.Id)
		}
		created, err := time.Parse(time.RFC3339, container.Created)
		if err != nil {
			return nil, err
		}
		if created.After(now.Add(-*ageContainer)) {
			continue
		}
		debug("  + creation before %s", *ageContainer)
		if container.State.FinishedAt.After(now.Add(-*ageContainer)) {
			continue
		}
		debug("  + exited before %s", *ageContainer)
		oldContainers = append(oldContainers, container)
	}

	expiredContainers := []*dockerclient.ContainerInfo{}
	for _, c := range oldContainers {
		name := c.Name[1:] // Remove leading /
		if usedVolumeContainers[name] {
			continue
		}
		expiredContainers = append(expiredContainers, c)
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
