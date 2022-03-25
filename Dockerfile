# Build the manager binary
FROM golang:1.16 as builder

ARG GO_ARCH=amd64

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY main.go main.go
COPY api/ api/
COPY controllers/ controllers/
COPY utils/ utils/

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=$GO_ARCH GO111MODULE=on go build -ldflags="-s -w" -a -o manager main.go


# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM registry.access.redhat.com/ubi8/ubi-minimal:latest

LABEL vendor="Open Liberty" \
      name="Open Liberty Operator" \
      version="0.8.1" \
      summary="Image for Open Liberty Operator" \
      description="This image contains the controllers for Open Liberty Operator. See https://github.com/OpenLiberty/open-liberty-operator#open-liberty-operator"


COPY LICENSE /licenses/
WORKDIR /
COPY --from=builder /workspace/manager .

USER 65532:65532

ENTRYPOINT ["/manager"]
