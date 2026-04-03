FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o replicast ./cmd/replicast

FROM alpine:3.20
WORKDIR /app
COPY --from=builder /app/replicast .
COPY --from=builder /app/migrations ./migrations
EXPOSE 8080
CMD ["./replicast"]