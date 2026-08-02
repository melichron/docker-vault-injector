# syntax=docker/dockerfile:1

# The controller is a single static Go binary. A multi-stage build keeps the
# runtime image small and avoids shipping a compiler or shell next to a process
# that has access to the Docker manager socket.
FROM golang:1.26-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .

ARG TARGETOS=linux
ARG TARGETARCH
RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" \
    -o /out/docker-vault-injector ./cmd/docker-vault-injector

FROM scratch
ENV PATH=/bin

# Vault should use HTTPS in production. The CA bundle is needed for public CAs;
# private CAs can additionally be mounted and selected with VAULT_CACERT.
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/docker-vault-injector /bin/docker-vault-injector

ENTRYPOINT ["/bin/docker-vault-injector"]

