FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server
RUN mkdir -p /out/data/images

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/server /server
# Owned by the distroless nonroot user so a fresh volume mounted here is writable.
COPY --from=build --chown=65532:65532 /out/data /data
ENV MEDIA_DIR=/data/images
EXPOSE 8080
ENTRYPOINT ["/server"]
