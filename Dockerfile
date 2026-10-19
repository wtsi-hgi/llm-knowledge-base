# syntax=docker/dockerfile:1

# Container image for the MLWH MCP server, serving streamable HTTP.
#
# Build:  docker build --build-arg VERSION=vX.Y.Z -t mlwh-mcp-server .
# Run:    docker run -e MLWH_BASE_URL=http://mlwh.internal:8080 -p 8080:8080 mlwh-mcp-server
#
# All configuration is by environment variable; see the README's Configuration
# table.

FROM golang:1.27 AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY cmd/ cmd/
COPY internal/ internal/

# Server version baked into the binary, as the Makefile's VERSION does. CI
# passes the git tag (e.g. vX.Y.Z) so --version matches a `make build` there.
ARG VERSION=dev

# The binary needs no cgo, so build it static for the distroless runtime;
# CGO_ENABLED=0 selects the pure-Go net and os/user implementations.
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags "-X github.com/wtsi-hgi/llm-knowledge-base/internal/core.ServerVersion=${VERSION}" \
    -o /out/mlwh-mcp-server ./cmd/mlwh-mcp-server

FROM gcr.io/distroless/static-debian13:nonroot

COPY --from=build /out/mlwh-mcp-server /usr/local/bin/mlwh-mcp-server

# Numeric UID so Kubernetes runAsNonRoot can verify it (distroless "nonroot").
USER 65532:65532

# A container is useful only in HTTP mode, so listen on all interfaces by
# default. MLWH_BASE_URL has no default and must be set at run time.
ENV MLWH_HTTP_ADDR=0.0.0.0:8080
EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/mlwh-mcp-server"]
