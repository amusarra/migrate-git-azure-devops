# =====================
# STAGE 1: Build
# =====================
FROM golang:1.23-alpine AS builder

# Variabili per compilazione statica
ENV CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64

WORKDIR /src

# Copia go.mod e go.sum per caching dipendenze
COPY go.mod go.sum ./
RUN go mod download

# Copia il resto del progetto
COPY . .

# Compila il binario
RUN go build -o /bin/migrazione-git-azure-devops ./cmd/migrazione-git-azure-devops

# =====================
# STAGE 2: Runtime
# =====================
FROM alpine:3.20 AS runtime

# Aggiungi certificati SSL (per client HTTP/HTTPS)
RUN apk --no-cache add ca-certificates

WORKDIR /app

# Copia solo il binario
COPY --from=builder /bin/migrazione-git-azure-devops .

# Comando di avvio
ENTRYPOINT ["./migrazione-git-azure-devops"]