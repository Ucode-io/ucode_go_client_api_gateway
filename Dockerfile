FROM golang:1.21.6 as builder

#
RUN mkdir -p $GOPATH/src/gitlab.udevs.io/ucode/ucode_go_client_api_gateway 
WORKDIR $GOPATH/src/gitlab.udevs.io/ucode/ucode_go_client_api_gateway

# Copy the local package files to the container's workspace.
COPY . ./

# installing depends and build
RUN export CGO_ENABLED=0 && \
    export GOOS=linux && \
    go mod vendor && \
    make build && \
    mv ./bin/ucode_go_client_api_gateway /

FROM alpine
COPY --from=builder ucode_go_client_api_gateway .
ENTRYPOINT ["/ucode_go_client_api_gateway"]