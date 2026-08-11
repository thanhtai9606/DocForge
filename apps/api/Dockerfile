# syntax=docker/dockerfile:1

FROM golang:1.22-alpine AS builder
RUN apk add --no-cache ca-certificates git
WORKDIR /src

COPY apps/api/go.mod apps/api/go.sum ./apps/api/
WORKDIR /src/apps/api
RUN go mod download

COPY apps/api/ ./
ARG TARGETOS=linux
ARG TARGETARCH=amd64
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/docforge-api ./cmd/api

FROM alpine:3.20 AS runtime
RUN apk add --no-cache ca-certificates tzdata \
    && adduser -D -H -u 10001 docforge
WORKDIR /app
COPY --from=builder /out/docforge-api /app/docforge-api
USER docforge
EXPOSE 8080
ENTRYPOINT ["/app/docforge-api"]
