# Temporal serverless (AWS Lambda) worker image for tmprl-demo.cloud.
#
# The platform builds this image, then wraps it with a Lambda launcher that
# materializes Temporal credentials (TEMPORAL_ADDRESS / TEMPORAL_NAMESPACE /
# TEMPORAL_API_KEY / TLS) and Worker Deployment env vars into the environment
# and then execs this image's entrypoint. The worker binary uses
# go.temporal.io/sdk/contrib/aws/lambdaworker, which drives the AWS Lambda
# runtime interface and polls the serverless-webinar task queue on each
# invocation, so no separate Lambda handler is needed here.
#
# Build context is the repo root: lambda-worker/go.mod references the shared
# module via `replace => ../shared`, so both directories must be copied in.
FROM --platform=linux/amd64 golang:1.24-alpine AS builder

WORKDIR /build

# Shared module first (referenced via the replace directive in go.mod).
COPY shared/ ../shared/

# Lambda worker module.
COPY lambda-worker/ .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o worker .

# --- Final image ---
FROM --platform=linux/amd64 alpine:3.19

WORKDIR /app

COPY --from=builder /build/worker .

ENTRYPOINT ["./worker"]
