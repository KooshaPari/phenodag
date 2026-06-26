# =============================================================================
# Dockerfile — multi-stage build for phenodag
# =============================================================================
# Stage 1: build
FROM golang:1.26-alpine AS builder

RUN apk add --no-cache gcc musl-dev

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 go build -mod=mod -ldflags="-s -w" -trimpath -o /phenodag .

# =============================================================================
# Stage 2: minimal runtime image
FROM alpine:3.21 AS runtime

RUN apk add --no-cache ca-certificates tzdata
RUN adduser -D -h /home/phenodag phenodag

COPY --from=builder /phenodag /usr/local/bin/phenodag

USER phenodag
WORKDIR /home/phenodag

# HEALTHCHECK requires the `health` subcommand.
# The --db path must be either a persistent mount or the default.
VOLUME [ "/home/phenodag/data" ]
ENV PHENODAG_DB=/home/phenodag/data/phenodag.db

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
  CMD phenodag health --db "$PHENODAG_DB"

EXPOSE 9090

ENTRYPOINT ["phenodag"]
CMD ["--help"]
