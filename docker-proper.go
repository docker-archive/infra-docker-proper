package main

import (
	"flag"
	"log"
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

		expiredContainers, expiredImages, err := getExpired(client)
		if err != nil {
			log.Fatal(err)
		}
		if err := removeContainers(client, expiredContainers); err != nil {
			log.Fatal(err)
		}

		if err := removeImages(client, expiredImages); err != nil {
			log.Fatal(err)
		}

		if *interval == 0 {
			break
		}
		log.Printf("Sleeping for %s", *interval)
		time.Sleep(*interval)
	}
}

func getExpired(client *dockerclient.DockerClient) (expiredContainers []*dockerclient.ContainerInfo, expiredImages []*dockerclient.Image, err error) {
	// Containers
	containers, err := client.ListContainers(true, false, "") // true = all containers
	if err != nil {
		return nil, nil, err
	}

	usedVolumeContainers := map[string]int{}
	usedImages := map[string]int{}
	oldContainers := []*dockerclient.ContainerInfo{}
	for _, c := range containers {
		debug("< container: %s", c.Id)
		container, err := client.InspectContainer(c.Id)
		if err != nil {
                        debug("Couldn't inspect container %s, skipping: %s", c.Id, err)
                        continue
		}

		// Increment reference counter refering to how many containers use volume container and image
		if len(container.HostConfig.VolumesFrom) > 0 {
			for _, vc := range container.HostConfig.VolumesFrom {
				usedVolumeContainers[vc]++
			}
		}
		debug("Container %s uses image %s", c.Id, container.Image)
		usedImages[container.Image]++

		if container.State.Running {
                        continue
		}
		debug("  + not running")

		if container.State.FinishedAt == time.Date(1, 1, 1, 0, 0, 0, 0, time.UTC) && !*unsafe {
                        debug("Container %s has empty FinishedAt field, skipping", c.Id)
                        continue
		}
		created, err := time.Parse(time.RFC3339, container.Created)
		if err != nil {
			return nil, nil, err
		}
		if created.After(now.Add(-*ageContainer)) {
                        continue
		}
		debug("  + creation time is older than %s", *ageContainer)
		if container.State.FinishedAt.After(now.Add(-*ageContainer)) {
                        continue
		}
		debug("  + exit time is older than %s", *ageContainer)

		// Decrement reference counter for old containers
		if len(container.HostConfig.VolumesFrom) > 0 {
			for _, vc := range container.HostConfig.VolumesFrom {
				usedVolumeContainers[vc]--
			}
		}
		usedImages[container.Image]--

		oldContainers = append(oldContainers, container)
	}

	for _, c := range oldContainers {
		name := c.Name[1:] // Remove leading /
		if usedVolumeContainers[name] > 0 {
			continue
		}
		expiredContainers = append(expiredContainers, c)
	}
	log.Printf("Found %d expired containers", len(expiredContainers))

	// Images
	images, err := client.ListImages(true)
	if err != nil {
		return nil, nil, err
	}

	for _, image := range images {
		debug("< image id: %s", image.Id)
		ctime := time.Unix(image.Created, 0)
		if ctime.After(now.Add(-*ageImage)) {
			continue
		}
		debug("  + older than %s", *ageImage)
		if usedImages[image.Id] > 0 {
			debug("  + in use, skipping")
			continue
		}
		expiredImages = append(expiredImages, image)
	}
	log.Printf("Found %d expired images", len(expiredImages))
	return expiredContainers, expiredImages, nil
}

func removeImages(client *dockerclient.DockerClient, images []*dockerclient.Image) error {
	for _, image := range images {
		log.Print("rmi ", image.Id)
		if !*dry {
			if _, err := client.RemoveImage(image.Id); err != nil {
				debug("Couldn't remove image: %s", err)
			}
		}
	}
	return nil
}

func removeContainers(client *dockerclient.DockerClient, containers []*dockerclient.ContainerInfo) error {
	for _, container := range containers {
		log.Printf("rm %s (%s)", container.Id, container.Name)
		if !*dry {
			if err := client.RemoveContainer(container.Id, false, false); err != nil {
				debug("Couldn't remove container: %s", err)
			}
		}
	}
	return nil
}
