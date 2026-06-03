FROM --platform=linux/amd64 golang:1.24-alpine AS builder

WORKDIR /build

# Copy shared module first (referenced via replace directive)
COPY shared/ ../shared/

# Copy demo-app module
COPY demo-app/ .

RUN go build -o demo-app .

# --- Final image ---
FROM --platform=linux/amd64 alpine:3.19

WORKDIR /app

COPY --from=builder /build/demo-app .

EXPOSE 8080

ENTRYPOINT ["./demo-app"]
