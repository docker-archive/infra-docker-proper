FROM       golang:alpine
MAINTAINER Brian Glogower <bglogower@docker.com> (@xbglowx)

RUN apk update && apk add git
RUN mkdir -p /go/src/app
WORKDIR /go/src/app

COPY . /go/src/app
RUN go-wrapper download
RUN go-wrapper install

ENTRYPOINT ["go-wrapper", "run"]
