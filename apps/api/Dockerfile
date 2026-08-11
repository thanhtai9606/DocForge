# Build from repository root:
#   docker build -f apps/api/Dockerfile -t docforge-api .
FROM golang:1.22-alpine AS build
WORKDIR /src

RUN apk add --no-cache ca-certificates git

COPY packages/cdom/go.mod ./packages/cdom/
COPY apps/api/go.mod apps/api/go.sum ./apps/api/
WORKDIR /src/apps/api
RUN go mod download

WORKDIR /src
COPY packages/cdom ./packages/cdom
COPY apps/api ./apps/api

WORKDIR /src/apps/api
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/api /api
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/api"]
