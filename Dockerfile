# syntax=docker/dockerfile:1
#
# Builds the foundry binary and packages it as a minimal container. The default
# entrypoint runs the gateway controller, but any foundry subcommand can be run
# by overriding the container args.
#
# Build context is the repository root:
#   docker build -t foundry-gateway-controller \
#     --build-arg VERSION=$(git describe --tags --always --dirty) \
#     --build-arg GIT_COMMIT=$(git rev-parse --short HEAD) \
#     --build-arg BUILD_DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ) .

FROM golang:1.25 AS build

WORKDIR /src/v1

# Download modules first so they are cached independently of source changes.
COPY v1/go.mod v1/go.sum ./
RUN go mod download

# Build the static binary with the same ldflags as ./tools build-static.
COPY v1/ ./
ARG VERSION=dev
ARG GIT_COMMIT=unknown
ARG BUILD_DATE=unknown
RUN CGO_ENABLED=0 go build \
    -ldflags "-X main.Version=${VERSION} -X main.GitCommit=${GIT_COMMIT} -X main.BuildDate=${BUILD_DATE} -s -w" \
    -o /out/foundry ./cmd/foundry

# Minimal, non-root runtime. The static distroless image has no shell or
# package manager, which keeps the attack surface small.
FROM gcr.io/distroless/static:nonroot

COPY --from=build /out/foundry /usr/local/bin/foundry

USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/foundry"]
CMD ["gateway", "controller"]
