package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/samalba/dockerclient"
)

const (
	dockerAPIVersion          = "/v1.10"
	expectedDeletedContainers = 1
	expectedDeletedImages     = 4
)

var (
	client *dockerclient.DockerClient
	docker *httptest.Server

	serverT *testing.T

	matchContainersURL = regexp.MustCompile("^" + dockerAPIVersion + "/containers/([^/]+)")
	matchImagesURL     = regexp.MustCompile("^" + dockerAPIVersion + "/images/([^/]+)")
	deletedContainers  int
	deletedImages      int
)

func fixture(w http.ResponseWriter, file string) {
	fh, err := os.Open("fixtures/" + file)
	if err != nil {
		serverT.Fatal(err)
	}
	defer fh.Close()
	if _, err := io.Copy(w, fh); err != nil {
		serverT.Fatal(err)
	}
}

func handleDockerAPI(w http.ResponseWriter, r *http.Request) {
	switch {
	case strings.HasPrefix(r.URL.Path, dockerAPIVersion+"/containers/json"):
		fixture(w, "containers.json")
		return
	case strings.HasPrefix(r.URL.Path, dockerAPIVersion+"/containers/"):
		matches := matchContainersURL.FindStringSubmatch(r.URL.Path)
		if len(matches) != 2 {
			serverT.Fatal("ID not found in url ", r.URL.Path)
		}
		switch r.Method {
		case "GET":
			fixture(w, "containers/"+matches[1]+".json")
		case "DELETE":
			deletedContainers++
			fmt.Fprintf(w, "")
		}
		return
	case strings.HasPrefix(r.URL.Path, dockerAPIVersion+"/images/json"):
		fixture(w, "images.json")
		return
	case strings.HasPrefix(r.URL.Path, dockerAPIVersion+"/images"):
		matches := matchImagesURL.FindStringSubmatch(r.URL.Path)
		if len(matches) != 2 {
			serverT.Fatal("ID not found in url ", r.URL.Path)
		}
		deletedImages++
		return
	}
	serverT.Fatalf("Unexpected request: %s", r.URL.Path)
	return
}

func TestExpired(t *testing.T) {
	serverT = t
	flag.Set("u", "true")

	docker = httptest.NewServer(http.HandlerFunc(handleDockerAPI))
	defer docker.Close()

	client, err := dockerclient.NewDockerClient(docker.URL, nil)
	if err != nil {
		t.Fatal(err)
	}

	expiredContainers, err := getExpiredContainers(client)
	if err != nil {
		t.Fatal(err)
	}

	if len(expiredContainers) != expectedDeletedContainers {
		t.Fatalf("Expected to delete %d container but deleted %d", expectedDeletedContainers, deletedContainers)
	}
}

func TestExpiredFail(t *testing.T) {
	flag.Set("u", "false")

	docker = httptest.NewServer(http.HandlerFunc(handleDockerAPI))
	defer docker.Close()

	client, err := dockerclient.NewDockerClient(docker.URL, nil)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := getExpiredContainers(client); err == nil {
		t.Fatal("Expected cleanup to fail because missing FinishedAt field")
	}
}
