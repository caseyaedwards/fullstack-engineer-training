# region: build
# Stage 1, build: the Go toolchain compiles both programs. Set GO_VERSION
# to at least the go line of go.mod, without the patch number.
ARG GO_VERSION=1.24
FROM golang:${GO_VERSION}-alpine AS build
WORKDIR /src

# Copy go.mod, and go.sum once it exists, and download the modules in a
# layer of their own. Docker reuses this layer until those files change.
COPY go.* ./
RUN go mod download

# Copy the rest of the source and build four static binaries into /out.
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/ ./cmd/api ./cmd/worker ./cmd/scheduler ./cmd/alerter
# endregion: build

# region: runtime
# Stage 2, runtime: an empty image, the CA certificates that HTTPS checks
# need, and a user ID that is not root.
FROM scratch AS runtime
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
USER 65532:65532
# endregion: runtime

# region: targets
# One final image per program. compose.yaml picks one with target:.
FROM runtime AS api
COPY --from=build /out/api /api
EXPOSE 8080
ENTRYPOINT ["/api"]

FROM runtime AS worker
COPY --from=build /out/worker /worker
ENTRYPOINT ["/worker"]

FROM runtime AS scheduler
COPY --from=build /out/scheduler /scheduler
ENTRYPOINT ["/scheduler"]

FROM runtime AS alerter
COPY --from=build /out/alerter /alerter
ENTRYPOINT ["/alerter"]
# endregion: targets

# region: web
# Week 10. Stage web-build: Node builds the page into static files. The
# package files come first, in a layer of their own, as go.mod does above.
FROM node:22-alpine AS web-build
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# The web image: nginx serves the files in dist/ and passes /api/ on to
# the api. This nginx image runs as user 101, not root, and listens on 8080.
FROM nginxinc/nginx-unprivileged:1.30-alpine-slim AS web
COPY web/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=web-build /web/dist /usr/share/nginx/html
EXPOSE 8080
# endregion: web
