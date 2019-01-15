FROM golang:1.9-alpine as builder

RUN apk update && \
    apk add --no-cache protobuf && \
    apk add --no-cache git

WORKDIR /go/src/github.com/carbonblack/cb-event-forwarder
COPY . .

# disable CGO (to help ensure no libc, can cause versioning problems across distros)
ENV CGO_ENABLED 0
# Static build flags - put these into the "go build" command below
# ENV STATIC_BUILD_FLAGS="-v -ldflags '-d -s -w' -a -tags netgo -installsuffix netgo"

RUN export GOPATH=/go && \
    export PATH=$PATH:$GOPATH/bin && \
    go get -u github.com/golang/protobuf/proto && \
    go get -u github.com/golang/protobuf/protoc-gen-go && \
    cd /go/src/github.com/carbonblack/cb-event-forwarder && \
    go generate ./... && \
    go get ./... && \
    go build -v -ldflags '-d -s -w' -a -tags netgo -installsuffix netgo

FROM golang:1.9-alpine
WORKDIR /
COPY --from=builder /go/bin/cb-event-forwarder .
COPY --from=builder /go/src/github.com/carbonblack/cb-event-forwarder/conf .
CMD ["go-wrapper", "run"]

# now you need to map in conf at /etc/cb/integrations/event-forwarder/cb-event-forwarder.conf
