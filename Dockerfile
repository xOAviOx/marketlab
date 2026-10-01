FROM node:22-alpine AS web
WORKDIR /src
RUN corepack enable
COPY web/package.json web/pnpm-lock.yaml ./web/
RUN pnpm --dir web install --frozen-lockfile
COPY web ./web
RUN pnpm --dir web build

FROM golang:1.23-alpine AS go
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./cmd/marketlab/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /marketlab ./cmd/marketlab

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=go /marketlab /marketlab
ENV PORT=8080
EXPOSE 8080
ENTRYPOINT ["/marketlab"]

