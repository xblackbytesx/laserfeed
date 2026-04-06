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

# Download Go dependencies (cached layer — only re-runs when go.mod changes)
COPY go.mod ./
RUN go mod download

# Copy source (go.sum excluded by .dockerignore so the downloaded one is preserved)
COPY . .

# Generate templ files first — web/templates/pages/*.go must exist before
# go mod tidy can recognise the package as local (not a missing remote module).
RUN go install github.com/a-h/templ/cmd/templ@v0.3.1001 && \
    templ generate

# Tidy ensures go.sum is complete for every package actually imported in source,
# guarding against stale cached go mod download layers.
RUN go mod tidy

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
