FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git nodejs npm

WORKDIR /app

# Front-end assets
COPY package.json ./
RUN npm install && \
    mkdir -p web/static/vendor/shoelace && \
    cp -r node_modules/@shoelace-style/shoelace/cdn/. web/static/vendor/shoelace/ && \
    cp node_modules/htmx.org/dist/htmx.min.js web/static/vendor/htmx.min.js && \
    rm -rf node_modules

# Go build
COPY go.mod ./
RUN go mod download

COPY . .

RUN go install github.com/a-h/templ/cmd/templ@v0.3.857 && \
    templ generate

RUN CGO_ENABLED=0 GOOS=linux go build -o laserfeed ./cmd/laserfeed

FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=builder /app/laserfeed .
COPY --from=builder /app/web/static ./web/static

EXPOSE 8080

CMD ["./laserfeed"]
