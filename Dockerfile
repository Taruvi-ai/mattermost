# Must match the server version built from this repo (server/public/model/version.go)
ARG MM_VERSION=11.7.11

# Stage 1: Build server from source
FROM golang:1.26 AS builder
SHELL ["/bin/bash", "-o", "pipefail", "-c"]
ARG MM_VERSION

# Build tools for server
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
    git gcc make curl \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /mattermost
COPY . .

WORKDIR /mattermost/server
ENV CI=true
# Without BUILD_NUMBER the Makefile builds a "dev" binary, which compiles in source-available
# enterprise code and forces developer mode
RUN make setup-go-work && make build-cmd-linux BUILD_NUMBER=${MM_VERSION}

# Stage 2: Build webapp from source
FROM node:24.11 AS webapp-builder
WORKDIR /mattermost/webapp
COPY webapp/ .
RUN npm ci && npm run build

# Stage 3: Download released Team Edition package (config + templates + prepackaged plugins)
FROM ubuntu:noble-20251013@sha256:c35e29c9450151419d9448b0fd75374fec4fff364a27f176fb458d472dfc9e54 AS release
SHELL ["/bin/bash", "-o", "pipefail", "-c"]
ARG MM_VERSION
ARG MM_PACKAGE="https://releases.mattermost.com/${MM_VERSION}/mattermost-team-${MM_VERSION}-linux-amd64.tar.gz"

RUN apt-get update \
    && DEBIAN_FRONTEND=noninteractive apt-get install --no-install-recommends -y \
    ca-certificates \
    curl \
    && rm -rf /var/lib/apt/lists/*

RUN mkdir -p /mattermost \
    && curl -L $MM_PACKAGE | tar -xvz \
    && rm -rf /mattermost/client

# Stage 4: Production runtime
FROM ubuntu:noble-20251013@sha256:c35e29c9450151419d9448b0fd75374fec4fff364a27f176fb458d472dfc9e54
SHELL ["/bin/bash", "-o", "pipefail", "-c"]

ARG PUID=2000
ARG PGID=2000

# Install runtime dependencies for document processing
RUN apt-get update \
    && DEBIAN_FRONTEND=noninteractive apt-get install --no-install-recommends -y \
    ca-certificates \
    curl \
    media-types \
    mailcap \
    unrtf \
    wv \
    poppler-utils \
    tidy \
    tzdata \
    tzdata-legacy \
    && rm -rf /var/lib/apt/lists/*

# Create mattermost user
RUN groupadd --gid ${PGID} mattermost \
    && useradd --uid ${PUID} --gid ${PGID} --comment "" --home-dir /mattermost mattermost

# Copy released package
COPY --from=release --chown=2000:2000 /mattermost /mattermost

# Webapp built from this repo
COPY --from=webapp-builder --chown=2000:2000 /mattermost/webapp/channels/dist /mattermost/client

# Replace server binaries with locally built ones
COPY --from=builder --chown=2000:2000 /mattermost/server/bin/mattermost /mattermost/bin/mattermost
COPY --from=builder --chown=2000:2000 /mattermost/server/bin/mmctl /mattermost/bin/mmctl

# Create required directories
RUN mkdir -p /mattermost/data /mattermost/logs /mattermost/plugins /mattermost/client/plugins /mattermost/.postgresql \
    && chmod 700 /mattermost/.postgresql \
    && chown -R mattermost:mattermost /mattermost

ENV PATH="/mattermost/bin:${PATH}"
ENV MM_SERVICESETTINGS_ENABLELOCALMODE="true"
ENV MM_INSTALL_TYPE="docker"

USER mattermost
WORKDIR /mattermost

HEALTHCHECK --interval=30s --timeout=10s \
    CMD ["/mattermost/bin/mmctl", "system", "status", "--local"]

EXPOSE 8065 8067 8074 8075

VOLUME ["/mattermost/data", "/mattermost/logs", "/mattermost/config", "/mattermost/plugins", "/mattermost/client/plugins"]

ENTRYPOINT ["/mattermost/bin/mattermost"]
