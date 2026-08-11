# syntax=docker/dockerfile:1
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/api ./cmd/api

# busybox provides wget for the HEALTHCHECK (distroless has no shell).
FROM busybox:1.36 AS busybox

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/api /api
COPY --from=busybox /bin/busybox /bin/busybox
ENV PORT=3000
EXPOSE 3000
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD ["/bin/busybox", "wget", "-q", "-O", "/dev/null", "http://127.0.0.1:3000/status"]
ENTRYPOINT ["/api"]