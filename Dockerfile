# Stage 1: Build frontend
FROM node:22-alpine AS frontend
WORKDIR /app/web
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# Stage 2: Build backend
FROM golang:1.24-alpine AS backend
RUN apk add --no-cache git
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /app/web/dist ./web/dist
RUN CGO_ENABLED=0 GOOS=linux go build -o /gitwise ./cmd/gitwise

# Stage 3: Runtime
FROM alpine:3.21
RUN apk add --no-cache git ca-certificates tzdata
COPY --from=backend /gitwise /usr/local/bin/gitwise
EXPOSE 3000
ENTRYPOINT ["gitwise"]
