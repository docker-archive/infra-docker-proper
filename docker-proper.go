package main

import (
	"flag"
	"log"
	"time"

	"github.com/samalba/dockerclient"
)

var (
	addr         = flag.String("a", "unix:///var/run/docker.sock", "address of docker daemon")
	ageContainer = flag.Duration("ca", 4*7*24*time.Hour, "Max container age")
	ageImage     = flag.Duration("ia", 4*7*24*time.Hour, "Max images age")
	interval     = flag.Duration("r", 0, "Run continously in given interval")

	dry     = flag.Bool("dry", false, "Dry run; do not actually delete")
	unsafe  = flag.Bool("u", false, "Unsafe; Delete container without FinishedAt field set")
	verbose = flag.Bool("v", false, "Be verbose")
	now     = time.Now()
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

		expiredContainers, err := getExpiredContainers(client)
		if err != nil {
			log.Fatal(err)
		}
		if err := removeContainers(client, expiredContainers); err != nil {
			log.Fatal(err)
		}

		if err := removeImages(client); err != nil {
			log.Fatal(err)
		}

		if *interval == 0 {
			break
		}
		log.Printf("Sleeping for %s", *interval)
		time.Sleep(*interval)
	}
}

func getExpiredContainers(client *dockerclient.DockerClient) (expiredContainers []*dockerclient.ContainerInfo, err error) {
	containers, err := client.ListContainers(true, false, "") // true = all containers
	if err != nil {
		return nil, err
	}

	usedVolumeContainers := map[string]int{}
	usedImages := map[string]int{}
	oldContainers := []*dockerclient.ContainerInfo{}
	for _, c := range containers {
		debug("< container: %s", c.Id)
		container, err := client.InspectContainer(c.Id)
		if err != nil {
			debug("  + Couldn't inspect container %s, skipping: %s", c.Id, err)
			continue
		}

		// Increment reference counter refering to how many containers use volume container and image
		if len(container.HostConfig.VolumesFrom) > 0 {
			for _, vc := range container.HostConfig.VolumesFrom {
				usedVolumeContainers[vc]++
			}
		}
		debug("  + Container %s uses image %s", c.Id, container.Image)
		usedImages[container.Image]++

		if container.State.Running {
			debug("  + Container is still running")
			continue
		}
		debug("  + not running")

		if container.State.FinishedAt == time.Date(1, 1, 1, 0, 0, 0, 0, time.UTC) && !*unsafe {
			debug("  + Container %s has empty FinishedAt field, skipping", c.Id)
			continue
		}
		created, err := time.Parse(time.RFC3339, container.Created)
		if err != nil {
			return nil, err
		}
		debug("  + Container and image threshold %s", now.Add(-*ageContainer))
		if created.After(now.Add(-*ageContainer)) {
			debug("  + Creation time is not old enough: %s", created)
			continue
		}
		debug("  + Creation time is older than %s", *ageContainer)
		if container.State.FinishedAt.After(now.Add(-*ageContainer)) {
			debug("  + Exit time is not old enough: %s", container.State.FinishedAt)
			continue
		}
		debug("  + Exit time is older than %s", *ageContainer)

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

	return expiredContainers, nil
}

// Search for all images that have expired based on operator input and try to
// remove them. If they are currently being used, the remvoal will fail, but that
// is OK.
func removeImages(client *dockerclient.DockerClient) error {
	expiredImages := []*dockerclient.Image{}
	images, err := client.ListImages(true)
	if err != nil {
		return err
	}

	for _, image := range images {
		debug("< image id: %s", image.Id)
		ctime := time.Unix(image.Created, 0)
		if ctime.After(now.Add(-*ageImage)) {
			continue
		}
		debug("  + older than %s", *ageImage)
		expiredImages = append(expiredImages, image)
	}

	log.Printf("Found %d expired images", len(expiredImages))

	for _, image := range expiredImages {
		log.Print("rmi ", image.Id)
		if !*dry {
			if _, err := client.RemoveImage(image.Id, false); err != nil {
				debug("Couldn't remove image with ID %s. Let's try to remove the same image, but this time using repo tags: %s", image.Id, err)
				for _, repoTag := range image.RepoTags {
					log.Print("rmi ", repoTag)
					if _, err := client.RemoveImage(repoTag, false); err != nil {
						debug("Couldn't remove image with repo tag %s: %s", repoTag, err)
					}
				}
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
