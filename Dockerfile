# Build the two binaries the service needs: the server, and the migrator that
# brings the schema up before it starts.
FROM golang:1.26-alpine AS build

WORKDIR /src

# Dependencies first. This layer is rebuilt only when go.mod or go.sum changes,
# so an ordinary code change does not re-download the module cache.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev

# CGO off so the result is a static binary with nothing to link against at run
# time. -trimpath keeps build machine paths out of it, and -s -w drop the debug
# tables, which is most of the size.
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/server ./cmd/server && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/migrate ./cmd/migrate

# distroless/static: certificates, a nonroot user, and nothing else. No shell, no
# package manager, no busybox. Everything the service serves is embedded in the
# binary, so there is nothing for a filesystem to hold.
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/server /server
COPY --from=build /out/migrate /migrate

EXPOSE 8080

# Already the image's default, and stated anyway so that a later edit has to
# decide to drop it rather than drop it by accident.
USER nonroot:nonroot

# No HEALTHCHECK on purpose. It would run inside the container, and there is no
# shell or curl here to run it with. Whatever schedules this probes /health over
# HTTP from outside, which is what compose and Kubernetes both do.
ENTRYPOINT ["/server"]
