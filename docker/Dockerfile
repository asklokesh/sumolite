# Linux-only. macOS hosts run the native binary because Docker on macOS
# can't reach VideoToolbox or the screen-capture APIs.
#
# Build:  docker build -f docker/Dockerfile -t sumolite .
# Run:    docker run --rm -it --network host --device /dev/dri \
#                    -v /run/user/1000:/run/user/1000 -e XDG_RUNTIME_DIR=/run/user/1000 \
#                    sumolite

FROM golang:1.22 AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 go build -ldflags="-s -w" -o /out/sumolite ./cmd/sumolite

FROM ubuntu:24.04
RUN apt-get update && apt-get install -y --no-install-recommends \
      gstreamer1.0-tools \
      gstreamer1.0-plugins-base gstreamer1.0-plugins-good \
      gstreamer1.0-plugins-bad  gstreamer1.0-plugins-ugly \
      gstreamer1.0-vaapi gstreamer1.0-pipewire \
      libva-drm2 libva2 \
      ca-certificates \
  && rm -rf /var/lib/apt/lists/*

COPY --from=build /out/sumolite /usr/local/bin/sumolite
COPY web/static /opt/sumolite/web/static
WORKDIR /opt/sumolite
EXPOSE 7878
ENTRYPOINT ["/usr/local/bin/sumolite", "serve"]
