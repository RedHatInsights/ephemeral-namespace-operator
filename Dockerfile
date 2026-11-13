FROM registry.access.redhat.com/hi/go:1.25-builder@sha256:e5a39920edca5c565aab1ffde285367de4d90ea9882663e8e474156fae95d487 AS build
WORKDIR /workspace
COPY --chown=65532:0 go.mod go.sum ./
RUN go mod download
COPY --chown=65532:0 main.go ./
COPY --chown=65532:0 apis/ apis/
COPY --chown=65532:0 controllers/ controllers/
COPY --chown=65532:0 config/ config/
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o manager main.go

FROM registry.access.redhat.com/hi/go:1.25@sha256:531ecf9be0e01b6d1c9f2172d194fb84149bff925aad1ff28c1de030620ab82e
COPY --from=build --chown=65532:0 /workspace/manager /manager
USER 65532:65532
ENTRYPOINT ["/manager"]
