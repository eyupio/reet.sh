# syntax=docker/dockerfile:1

# The UI and the Go binary are built on the build host's own architecture and
# cross-compiled, so multi-arch images need no emulation.
FROM --platform=$BUILDPLATFORM node:22-alpine AS web
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ARG COMMIT=unknown
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /web/dist ./web/dist
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT}" -o /out/reet ./cmd/reet \
 && mkdir -p /out/data

FROM gcr.io/distroless/static-debian12:nonroot
LABEL org.opencontainers.image.source="https://github.com/eyupio/reet.sh" \
      org.opencontainers.image.description="Like, bookmark and track the skills you use from skills.sh"
COPY --from=build /out/reet /reet
COPY --from=build --chown=65532:65532 /out/data /data
ENV REET_DATA_DIR=/data REET_ADDR=:8080
VOLUME /data
EXPOSE 8080
USER 65532:65532
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 CMD ["/reet", "healthcheck"]
ENTRYPOINT ["/reet"]
CMD ["serve"]
