FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/habitd ./cmd/habitd \
    && mkdir -p /out/data

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/habitd /habitd
COPY --from=build --chown=65532:65532 /out/data /data
ENV HABITD_DB=/data/habitd.db HABITD_RUNTIME=Docker
VOLUME /data
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s CMD ["/habitd", "healthcheck"]
ENTRYPOINT ["/habitd"]
