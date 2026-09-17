# Trusted-host credential-isolation sidecar (M4).
#
# Build from the repo root:
#   docker build -f sidecar/Dockerfile -t shoplazza-sidecar .
#
# The sidecar needs the real credentials at RUNTIME (config + keychain), which
# are MOUNTED, never baked into the image. Run it only on a trusted host.
FROM golang:1.23 AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/shoplazza-sidecar ./sidecar/server

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/shoplazza-sidecar /usr/local/bin/shoplazza-sidecar
# Real credentials live under the mounted home; nothing secret is in the image.
#   docker run --rm -p 16384:16384 \
#     -v "$HOME/.config/shoplazza:/home/nonroot/.config/shoplazza:ro" \
#     -v "$HOME/.shoplazza-sidecar:/home/nonroot/.shoplazza-sidecar" \
#     shoplazza-sidecar
EXPOSE 16384
ENTRYPOINT ["/usr/local/bin/shoplazza-sidecar"]
CMD ["--listen", "0.0.0.0:16384"]
