# syntax=docker/dockerfile:1

FROM golang:1.27.4-alpine3.24 AS construction
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
ARG VERSION=dev
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/coinchenet ./cmd/coinchenet

FROM gcr.io/distroless/static-debian13:nonroot
COPY --from=construction /out/coinchenet /coinchenet
USER nonroot:nonroot
EXPOSE 8080 9090
HEALTHCHECK --interval=10s --timeout=4s --start-period=15s --retries=3 CMD ["/coinchenet", "sante"]
ENTRYPOINT ["/coinchenet"]
CMD ["serveur"]
