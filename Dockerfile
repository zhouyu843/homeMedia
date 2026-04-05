FROM golang:1.24-alpine

WORKDIR /app

RUN apk add --no-cache ca-certificates git ffmpeg nodejs npm

COPY go.mod ./
RUN go mod download

CMD ["go", "run", "./cmd/server"]
