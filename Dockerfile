FROM       debian:jessie
MAINTAINER Johannes 'fish' Ziemke <fish@docker.com> (@discordianfish)

RUN        apt-get update && apt-get install -yq curl git
RUN        curl -s https://storage.googleapis.com/golang/go1.4.2.linux-amd64.tar.gz | tar -C /usr/local -xzf -
ENV        PATH    /usr/local/go/bin:$PATH
ENV        GOPATH  /go

ADD        . /docker-proper
WORKDIR    /docker-proper
RUN        go get -d && go build
ENTRYPOINT [ "./docker-proper" ]

