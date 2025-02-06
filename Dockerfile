# Build the manager binary
FROM registry.access.redhat.com/ubi8-minimal:latest as builder
ARG GO_PLATFORM=amd64
ARG GO_VERSION_ARG
ARG LIBERTY_VERSION=25.0.0.1
ENV PATH=$PATH:/usr/local/go/bin
RUN microdnf install tar gzip unzip

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum

RUN if [ -z "${GO_VERSION_ARG}" ]; then \
      GO_VERSION=$(grep '^go [0-9]\+.[0-9]\+' go.mod | cut -d ' ' -f 2); \
    else \
      GO_VERSION=${GO_VERSION_ARG}; \
    fi; \
    rm -rf /usr/local/go; \
    curl -L --output - "https://golang.org/dl/go${GO_VERSION}.linux-${GO_PLATFORM}.tar.gz" | tar -xz -C /usr/local/; \
    mkdir -p /opt/ol; \
    curl -L -o /opt/ol/wlp.zip "https://repo1.maven.org/maven2/io/openliberty/openliberty-kernel/${LIBERTY_VERSION}/openliberty-kernel-${LIBERTY_VERSION}.zip"; \
    unzip /opt/ol/wlp.zip; \
    rm -f /opt/ol/wlp.zip; \
    mkdir -p /opt/ol/wlp/output;


# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY cmd/main.go cmd/main.go
COPY api/ api/
COPY internal/controller/ internal/controller/
COPY utils/ utils/

# Build
RUN CGO_ENABLED=0 GOOS=linux GO111MODULE=on go build -ldflags="-s -w" -a -o manager cmd/main.go

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM registry.access.redhat.com/ubi8/openjdk-11:latest

ARG USER_ID=65532
ARG GROUP_ID=65532

ARG VERSION_LABEL=1.4.1
ARG RELEASE_LABEL=XX
ARG VCS_REF=0123456789012345678901234567890123456789
ARG VCS_URL="https://github.com/OpenLiberty/open-liberty-operator"
ARG NAME="open-liberty-operator"
ARG SUMMARY="Open Liberty Operator"
ARG DESCRIPTION="This image contains the controllers for Open Liberty Operator."

LABEL name=$NAME \
      vendor=IBM \
      version=$VERSION_LABEL \
      release=$RELEASE_LABEL \
      description=$DESCRIPTION \
      summary=$SUMMARY \
      io.k8s.display-name=$SUMMARY \
      io.k8s.description=$DESCRIPTION \
      vcs-type=git \
      vcs-ref=$VCS_REF \
      vcs-url=$VCS_URL \
      url=$VCS_URL

COPY --chown=${USER_ID}:${GROUP_ID} LICENSE /licenses/
WORKDIR /
COPY --from=builder --chown=${USER_ID}:${GROUP_ID} /workspace/manager .
COPY --from=builder --chown=${USER_ID}:${GROUP_ID} /workspace/internal/controller/assets/ /internal/controller/assets
COPY --from=builder --chown=${USER_ID}:${GROUP_ID} /workspace/opt/ol/wlp /opt/ol/wlp


ENTRYPOINT ["/manager"]
