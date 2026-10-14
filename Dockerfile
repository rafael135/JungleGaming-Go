# ==============================================================================
# Multi-stage Dockerfile para Jungle Gaming Backend
# ==============================================================================

# Estágio 1: Compilação do binário
FROM golang:alpine AS builder

WORKDIR /app

RUN apk add --no-cache git ca-certificates

# Cache de dependências
COPY go.mod go.sum ./
RUN go mod download

# Código-fonte
COPY . .

# Compilação estática sem CGO
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /app/bin/api ./cmd/api

# Estágio 2: Imagem final mínima e segura
FROM alpine:3.20

WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata curl

# Copia o binário compilado
COPY --from=builder /app/bin/api /app/api

EXPOSE 8081

ENTRYPOINT ["/app/api"]

