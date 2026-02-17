FROM golang:1.24-alpine AS builder
RUN apk add --no-cache git gcc musl-dev make
WORKDIR /build
# Create the proper directory structure for the workspace
RUN mkdir -p github.com/mattermost/mattermost/server/v8
RUN mkdir -p github.com/mattermost/mattermost/server/v8/public
ENV PATH="/mattermost/bin:${PATH}"
ENV MM_SERVICESETTINGS_ENABLELOCALMODE="true"
ENV MM_INSTALL_TYPE="docker"
# Copy the main server module
WORKDIR /build/github.com/mattermost/mattermost/server/v8
COPY server/go.mod server/go.sum ./
COPY server/ ./

# Copy the public module
WORKDIR /build/github.com/mattermost/mattermost/server/v8/public
COPY server/public/go.mod server/public/go.sum ./
COPY server/public/ ./

# Go back to the main server directory to build
WORKDIR /build/github.com/mattermost/mattermost/server/v8

# Add a replace directive to point to the local public module
RUN go mod edit -replace github.com/mattermost/mattermost/server/public=./public

RUN go mod download
RUN go build -o bin/mattermost cmd/mattermost/main.go

FROM alpine:latest
RUN apk add --no-cache ca-certificates tzdata bash curl
WORKDIR /mattermost

# Create mattermost user with UID 2000
RUN addgroup -g 2000 mattermost && \
    adduser -D -u 2000 -G mattermost mattermost

# Copy the binary and necessary files
COPY --from=builder /build/github.com/mattermost/mattermost/server/v8/bin/mattermost /mattermost/bin/mattermost
COPY --from=builder /build/github.com/mattermost/mattermost/server/v8/config/ /mattermost/config/
COPY --from=builder /build/github.com/mattermost/mattermost/server/v8/i18n/ /mattermost/i18n/
COPY --from=builder /build/github.com/mattermost/mattermost/server/v8/templates/ /mattermost/templates/
COPY --from=builder /build/github.com/mattermost/mattermost/server/v8/fonts/ /mattermost/fonts/
COPY --from=builder /build/github.com/mattermost/mattermost/server/v8/public/ /mattermost/public/

# Create necessary directories and set ownership
RUN mkdir -p /mattermost/data /mattermost/logs /mattermost/plugins /mattermost/client/plugins && \
    chown -R mattermost:mattermost /mattermost
ENV PATH="/mattermost/bin:${PATH}"
ENV MM_SERVICESETTINGS_ENABLELOCALMODE="true"
ENV MM_INSTALL_TYPE="docker"
EXPOSE 8065
USER mattermost
ENTRYPOINT ["./bin/mattermost"]
CMD ["server"]
