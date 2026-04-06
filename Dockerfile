FROM golang:1.24-alpine AS dev

WORKDIR /app

RUN apk add --no-cache ca-certificates git ffmpeg nodejs npm

COPY go.mod go.sum ./
RUN go mod download

CMD ["go", "run", "./cmd/server"]

FROM node:22-alpine AS frontend-builder

WORKDIR /src/web/frontend

COPY web/frontend/package*.json ./
RUN npm install

COPY web/frontend/ ./
RUN npm run build

FROM dev AS backend-builder

WORKDIR /src

COPY go.mod go.sum ./
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY --from=frontend-builder /src/web/static/app ./web/static/app

RUN CGO_ENABLED=0 GOOS=linux go build -o /out/homemedia ./cmd/server

FROM alpine:3.21

WORKDIR /app

RUN apk add --no-cache ca-certificates ffmpeg \
	&& mkdir -p /app/web/static/app /data/uploads

COPY --from=backend-builder /out/homemedia /app/homemedia
COPY --from=frontend-builder /src/web/static/app /app/web/static/app

EXPOSE 8080

CMD ["/app/homemedia"]
