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
    dry          = flag.Bool("dry", false, "Dry run; do not actually delete")
    unsafe       = flag.Bool("u", false, "Unsafe; Delete container without FinishedAt field set")
    verbose      = flag.Bool("v", false, "Be verbose")
    now          = time.Now()
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

        expiredContainers, usedImages, err := getExpiredContainers(client)
        if err != nil {
            log.Fatal(err)
        }

        if err := removeContainers(client, expiredContainers); err != nil {
            log.Fatal(err)
        }

        if err := removeImages(client, usedImages); err != nil {
            log.Fatal(err)
        }

        if *interval == 0 {
            break
        }
        log.Printf("Sleeping for %s", *interval)
        time.Sleep(*interval)
    }
}

func getExpiredContainers(client *dockerclient.DockerClient) (expiredContainers []*dockerclient.ContainerInfo, usedImages map[string][]string, err error) {
    ListAllContainers, err := client.ListContainers(true, false, "") // true = all containers
    if err != nil {
        return nil, nil, err
    }

    usedVolumeContainers := map[string]int{}
    usedImages = map[string][]string{}

    oldContainers := []*dockerclient.ContainerInfo{}
    for _, container := range ListAllContainers {
        debug("< container: %s", container.Id)
        containerInspect, err := client.InspectContainer(container.Id)
        if err != nil {
            debug("  + Couldn't inspect container %s, skipping: %s", container.Id, err)
            continue
        }

        // Keep track of images and which containers use them
        if containerNames, ok := usedImages[containerInspect.Image]; ok {
            containerNames = append(containerNames, containerInspect.Name)
            usedImages[containerInspect.Image] = containerNames
        } else {
            usedImages[containerInspect.Image] = []string{containerInspect.Name}
        }

        // Increment reference counter refering to how many containers use volume container
        if len(containerInspect.HostConfig.VolumesFrom) > 0 {
            for _, vc := range containerInspect.HostConfig.VolumesFrom {
                usedVolumeContainers[vc]++
            }
        }
        debug("  + Container %s uses image %s", container.Id, containerInspect.Image)

        if containerInspect.State.Running {
            debug("  + Container is still running")
            continue
        }
        debug("  + not running")

        if containerInspect.State.FinishedAt == time.Date(1, 1, 1, 0, 0, 0, 0, time.UTC) && !*unsafe {
            debug("  + Container %s has empty FinishedAt field, skipping", container.Id)
            continue
        }
        created, err := time.Parse(time.RFC3339, containerInspect.Created)
        if err != nil {
            return nil, nil, err
        }
        debug("  + Container and image threshold %s", now.Add(-*ageContainer))
        if created.After(now.Add(-*ageContainer)) {
            debug("  + Creation time is not old enough: %s", created)
            continue
        }
        debug("  + Creation time is older than %s", *ageContainer)
        if containerInspect.State.FinishedAt.After(now.Add(-*ageContainer)) {
            debug("  + Exit time is not old enough: %s", containerInspect.State.FinishedAt)
            continue
        }
        debug("  + Exit time is older than %s", *ageContainer)

        // Decrement reference counter for old containers
        if len(containerInspect.HostConfig.VolumesFrom) > 0 {
            for _, vc := range containerInspect.HostConfig.VolumesFrom {
                usedVolumeContainers[vc]--
            }
        }

        // Let's remove container reference from the image list, since said
        // container will soon be deleted.
        containerNames := usedImages[containerInspect.Image]
        containerNames = containerNames[:len(containerNames)-1]
        if len(containerNames) == 0 {
            delete(usedImages, containerInspect.Image)
        } else {
            usedImages[containerInspect.Image] = containerNames
        }
        oldContainers = append(oldContainers, containerInspect)
    }

    for _, oldContainer := range oldContainers {
        name := oldContainer.Name[1:] // Remove leading /
        if usedVolumeContainers[name] > 0 {
            continue
        }
        expiredContainers = append(expiredContainers, oldContainer)
    }

    log.Printf("Found %d expired containers", len(expiredContainers))
    return expiredContainers, usedImages, nil
}

// Search for all images that have expired based on operator input and try to
// remove them. If they are currently being used, the remvoal will fail, but that
// is OK.
func removeImages(client *dockerclient.DockerClient, usedImages map[string][]string) error {
    expiredImages := []*dockerclient.Image{}

    images, err := client.ListImages(true)
    if err != nil {
        return err
    }

    for _, image := range images {
        debug("< image id: %s", image.Id)
        // If image is still being used, let's skip it.
        if containers, ok := usedImages[image.Id]; ok {
            debug("  + skipping image %s, since it is still being used by %v.", image.Id, containers)
            continue
        }
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
