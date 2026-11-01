# syntax=docker/dockerfile:1

# Both build stages run on the build host; Go cross-compiles for the target.
FROM --platform=$BUILDPLATFORM node:24-alpine AS web
WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ARG COMMIT=unknown
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# .dockerignore keeps any local dist out; only the fresh build is embedded.
COPY --from=web /src/frontend/dist ./frontend/dist
RUN ./scripts/check-embed.sh
RUN CGO_ENABLED=0 go test ./...
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath \
      -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT}" \
      -o /out/mihomo-visz ./cmd/mihomo-visz \
 && mkdir -p /out/data

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/mihomo-visz /app/mihomo-visz
COPY --from=build --chown=nonroot:nonroot /out/data /app/data
VOLUME ["/app/data"]
EXPOSE 36750
ENTRYPOINT ["/app/mihomo-visz", "-config", "/app/config.yaml"]
