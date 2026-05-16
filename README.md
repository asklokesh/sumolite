# sumolite

Ultra-low-latency remote desktop. WebRTC under the hood, browser client on top, single static binary on the server. macOS (Apple Silicon + Intel) and Linux (Ubuntu 22.04+).

## Why not VNC?

VNC is a 1998 framebuffer protocol with no hardware encode, no adaptive bitrate, no congestion control, no GPU path. On a modern GPU it leaves 95% of the silicon idle and ships JPEG tiles over TCP.

## Why not Sunshine + Moonlight?

Sunshine + Moonlight is genuinely great. sumolite is aimed at the install/use axis:

| | Sunshine + Moonlight | sumolite |
|---|---|---|
| Client install | Native Moonlight app per device | Browser. Open URL. Done. |
| Pairing | PIN exchanged in two UIs | Short code typed once |
| Server install | Distro packages, manual cert/firewall | `curl ... \| sh`, or `docker run` |
| Update | Manual | `sumolite update` (self-update) |
| Transport | Custom NVENC over UDP (game-stream protocol) | WebRTC — congestion control, NACK, FEC, TURN fallback for NAT |
| Latency target | ~5-10ms encode + network | Same: hardware encode + WebRTC SRTP |

The latency floor is set by the encoder, not the protocol. Both use hardware H.264/H.265/AV1. sumolite wins on the "five minutes from zero to streaming on a Chromebook in a cafe" path.

## Quick start

### Server (one line)

```bash
curl -fsSL https://sumolite.dev/install.sh | sh
sumolite serve
```

Prints a pairing URL and a 6-digit code. Open the URL on any device, type the code, you're in. The code is saved to `~/.config/sumolite/token` and reused on restart, so bookmarked links keep working. Use `--rotate-token` to mint a fresh one.

### macOS first-run permissions

macOS gates screen capture behind TCC. The first time you run `sumolite serve` it will silently fail to produce video (the browser will show a black tab with a healthy RTT in the HUD). Grant Screen Recording permission to the `ffmpeg` binary:

1. Run `sumolite doctor` once. It prints the available displays and the path you need to allow.
2. Open **System Settings -> Privacy & Security -> Screen & System Audio Recording**, click **+**, hit **Cmd+Shift+G**, and paste the real ffmpeg path (resolve the symlink, e.g. `/opt/homebrew/Cellar/ffmpeg/8.1.1/bin/ffmpeg`). Enable the toggle.
3. Restart `sumolite serve`. The capture log will go from `0 bytes/s` to multi-Mbps within the first second.

The first browser click will also trigger an Accessibility prompt for mouse and keyboard injection. Approve it.

If your Mac has multiple displays, run `sumolite doctor` to see the indices and pass `--display 2` (or whatever) to pick a specific screen.

### Server (Docker, Linux only)

```bash
docker run --rm -it \
  --device /dev/dri \
  --network host \
  -e SUMOLITE_TOKEN=$(openssl rand -hex 16) \
  ghcr.io/asklokesh/sumolite:latest
```

### Client

Any modern browser. Chrome / Edge / Safari 17+ / Firefox.

## Build from source

```bash
make build       # native binary
make docker      # linux/amd64 + linux/arm64 images
make dev         # run locally with live web reload
```

Requires Go 1.22+ and GStreamer 1.22+ with the platform-specific plugins:

- **macOS**: `brew install gstreamer gst-plugins-base gst-plugins-good gst-plugins-bad`
- **Ubuntu**: `apt install gstreamer1.0-plugins-{base,good,bad,ugly} gstreamer1.0-vaapi`

## Capture + encode pipelines

Picked at runtime based on what's available:

- **macOS (Apple Silicon)**: `avfvideosrc capture-screen=true ! vtenc_h264_hw realtime=true allow-frame-reordering=false max-keyframe-interval=120`
- **macOS (Intel)**: same, falls back to `vtenc_h264` if hw encoder is busy
- **Linux + Intel/AMD**: `pipewiresrc ! vah264enc rate-control=cbr` (Wayland) or `ximagesrc ! vah264enc` (X11)
- **Linux + NVIDIA**: `pipewiresrc ! nvh264enc preset=low-latency-hp rc-mode=cbr`
- **Linux fallback**: `ximagesrc ! x264enc tune=zerolatency speed-preset=ultrafast` (software, only if no GPU encode available)

Keyframe interval is intentionally long; recovery is via WebRTC PLI/NACK instead of periodic IDR.

## Input injection

- **macOS**: CGEvent via cgo
- **Linux/Wayland**: `libei` (emulated input over the portal)
- **Linux/X11**: XTest

## Status

Greenfield. The scaffold compiles, signaling and the browser client connect, the GStreamer pipeline is wired. Input injection on macOS works; on Linux/Wayland it requires the portal grant on first run. AV1 path is gated behind a flag.

## License

MIT.
