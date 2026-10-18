# syntax=docker/dockerfile:1.7
FROM node:22-alpine AS web
WORKDIR /src/web
ENV CI=true PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1
RUN corepack enable && corepack prepare pnpm@10.28.0 --activate
COPY web/package.json web/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY web/ ./
RUN mkdir -p /src/internal/web && pnpm build            # → /src/internal/web/dist

FROM golang:1.24-alpine AS build
WORKDIR /src
ENV CGO_ENABLED=0 GOOS=linux GOTOOLCHAIN=local
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY internal/ internal/
COPY migrations/ migrations/
COPY --from=web /src/internal/web/dist/ internal/web/dist/
ARG VERSION=dev
RUN go build -trimpath -ldflags="-s -w -X github.com/marquinhosvideo/processosplatdistr/internal/buildinfo.Version=${VERSION}" \
    -o /out/server ./cmd/server

FROM alpine:3.22
RUN apk add --no-cache ca-certificates && adduser -D -H -u 10001 app
# CA interna (IdP/DBaaS), se necessário: COPY certs/*.crt /usr/local/share/ca-certificates/ && update-ca-certificates
WORKDIR /home/application/current
COPY --from=build /out/server /usr/local/bin/server
COPY Procfile tsuru.yaml ./
USER 10001
EXPOSE 8888
CMD ["/usr/local/bin/server", "serve"]
