FROM golang:1.26-alpine AS builder

RUN apk add --no-cache git nodejs npm

WORKDIR /app

# Front-end assets
COPY web/package.json ./web/
RUN npm install --prefix ./web && \
    mkdir -p web/static/vendor/shoelace && \
    cp -r web/node_modules/@shoelace-style/shoelace/cdn/. web/static/vendor/shoelace/ && \
    cp web/node_modules/htmx.org/dist/htmx.min.js web/static/vendor/htmx.min.js && \
    rm -rf web/node_modules

# Download Go dependencies (cached layer — re-runs only when go.mod/go.sum change).
# Copying the committed go.sum lets `go mod download` verify checksums against it,
# rather than silently accepting whatever resolves at build time.
COPY go.mod go.sum ./
RUN go mod download

# Copy source.
COPY . .

RUN go install github.com/a-h/templ/cmd/templ@v0.3.1020 && \
    templ generate

RUN CGO_ENABLED=0 GOOS=linux go build -o laserfeed ./cmd/laserfeed

FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata && \
    addgroup -g 1000 laserfeed && \
    adduser -u 1000 -G laserfeed -D -H laserfeed

WORKDIR /app

COPY --from=builder /app/laserfeed .
COPY --from=builder /app/web/static ./web/static

RUN chown -R laserfeed:laserfeed /app

USER laserfeed

EXPOSE 8080

CMD ["./laserfeed"]
