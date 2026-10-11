# The Dockerfile of the Containers track. Build and run it from this
# folder:
#
#   docker build -t hello .
#   docker run --rm hello

# region: build
FROM golang:1.24 AS build
WORKDIR /src
COPY hello/go.mod ./
RUN go mod download
COPY hello/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/hello .
# endregion: build

# region: runtime
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/hello /hello
USER 65532:65532
ENTRYPOINT ["/hello"]
# endregion: runtime
