FROM golang:1.8

RUN apt-get update && \
    apt-get install -y libprotobuf-java protobuf-compiler

WORKDIR /go/src/github.com/carbonblack/cb-event-forwarder
COPY . .

RUN export GOPATH=/go && \
    export PATH=$PATH:$GOPATH/bin && \
    go get -u github.com/golang/protobuf/proto && \
    go get -u github.com/golang/protobuf/protoc-gen-go && \
    cd /go/src/github.com/carbonblack/cb-event-forwarder && \
    go generate ./... && \
    go get ./... && \
    go build

CMD ["go-wrapper", "run"]

# now you need to map in conf at /etc/cb/integrations/event-forwarder/cb-event-forwarder.conf