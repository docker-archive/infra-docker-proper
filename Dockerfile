FROM       debian:jessie
MAINTAINER Johannes 'fish' Ziemke <fish@docker.com> (@discordianfish)

RUN        apt-get update && apt-get install -yq curl git
RUN        curl -s https://go.googlecode.com/files/go1.2.linux-amd64.tar.gz | tar -C /usr/local -xzf -
ENV        PATH    /usr/local/go/bin:$PATH
ENV        GOPATH  /go
RUN        git clone -b some-additions https://github.com/discordianfish/dockerclient.git \
           /go/src/github.com/samalba/dockerclient

ADD        . /docker-proper
WORKDIR    /docker-proper
RUN        go get -d && go build
ENTRYPOINT [ "./docker-proper" ]

