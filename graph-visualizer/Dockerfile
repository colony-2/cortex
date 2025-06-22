# Build stage for frontend
FROM node:20-alpine AS frontend-builder
WORKDIR /app/web
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# Build stage for Go binary
FROM golang:1.21-alpine AS go-builder
RUN apk add --no-cache git
WORKDIR /app
COPY server/go.mod server/go.sum ./server/
RUN cd server && go mod download
COPY server/ ./server/
COPY --from=frontend-builder /app/web/dist ./web/dist
RUN cd server && go build -tags prod -o ../graph-visualizer .

# Final stage
FROM alpine:latest
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=go-builder /app/graph-visualizer .
COPY example/ ./example/

EXPOSE 8080
CMD ["./graph-visualizer", "-path=./example"]