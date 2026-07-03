# Stage 1: Build
FROM golang:1.26-bookworm AS builder

# Install templ
RUN go install github.com/a-h/templ/cmd/templ@latest

# Install tailwindcss CLI
RUN curl -sLO https://github.com/tailwindlabs/tailwindcss/releases/download/v4.3.2/tailwindcss-linux-x64 && \
    chmod +x tailwindcss-linux-x64 && \
    mv tailwindcss-linux-x64 /usr/local/bin/tailwindcss

WORKDIR /app

# Copy dependency files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Generate assets and build
RUN go generate ./...
RUN CGO_ENABLED=0 go build -o mailaroo cmd/mailaroo/*.go

# Create non-root user for runtime
RUN groupadd -r mailaroo && useradd -r -g mailaroo mailaroo

# Stage 2: Runtime
FROM gcr.io/distroless/static-debian12

COPY --from=builder /etc/ssl/certs /etc/ssl/certs
COPY --from=builder /etc/passwd /etc/passwd
COPY --from=builder /etc/group /etc/group

WORKDIR /app

# Copy binary and assets
COPY --from=builder --chown=mailaroo:mailaroo /app/mailaroo .
COPY --from=builder --chown=mailaroo:mailaroo /app/static ./static
COPY --from=builder --chown=mailaroo:mailaroo /app/db/migrations ./db/migrations

USER mailaroo

# SMTP (plain + submission + submissions) + Web UI
EXPOSE 25 465 587 8080

# The root command of the binary starts the server by default
ENTRYPOINT ["./mailaroo"]
