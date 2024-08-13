# Build the manager binary
FROM registry.access.redhat.com/ubi8/go-toolset:1.21.11-1.1720406008 as builder

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY main.go main.go
COPY apis/ apis/
COPY controllers/ controllers/
COPY config/ config/

USER 0

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o manager main.go

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM registry.access.redhat.com/ubi8/ubi-minimal:8.10-1052
WORKDIR /
COPY --from=builder /workspace/manager .
USER 65534:65534

ENTRYPOINT ["/manager"]
