# temporal-serverless-no-roads

> *We Don't Need Workers Where We're Going*

A live audience-participation demo for Temporal's Serverless Workers feature. Attendees submit their name via a web UI to trigger real workflow executions, and watch Lambda invocations, task queue backlog, and workflow counts update in real time.

## Repo structure

```
temporal-serverless-no-roads/
├── shared/               # Shared Go module — workflow, activity, task queue name, worker config
│   ├── activities/       # Activity implementations (ProcessSubmission, SimulateWork1/2/3)
│   ├── taskqueue/        # Shared task queue name constant
│   ├── workerconfig/     # Reads worker concurrency from environment variables
│   └── workflows/        # DemoWorkflow definition
├── lambda-worker/        # Deployable 1: Lambda worker (Go)
│   └── .env              # Documents Lambda environment variables (not bundled in zip)
├── demo-app/             # Deployable 2: HTTP server — UI + API (Go)
│   ├── api/              # /api/submit, /api/metrics, /api/seed handlers
│   ├── cache/            # Short-TTL metrics cache
│   ├── frontend/         # Embedded HTML UI
│   ├── localworker/      # Long-polling worker for local dev (not deployed)
│   │   └── .env          # Worker concurrency settings for local dev
│   ├── middleware/       # Per-IP rate limiter
│   └── k8s/              # Kubernetes manifests for EKS deployment
├── go.work               # Go workspace — ties all three modules together
└── README.md
```

---

## Running locally

Local dev uses the Temporal CLI dev server in place of Temporal Cloud, and a
standard Go worker in place of the Lambda worker. No AWS account required.

### Prerequisites

- [Go 1.22+](https://go.dev/dl/)
- [Temporal CLI](https://docs.temporal.io/cli#installation)

```bash
# macOS
brew install temporal

# Linux — check your arch and download from:
# https://temporal.download/cli/archive/latest?platform=linux&arch=amd64
```

### 1. Resolve dependencies

Run `go mod tidy` in each module (the workspace root has no `go.mod`, so you
need to do this per-module):

```bash
cd shared        && go mod tidy && cd ..
cd lambda-worker && go mod tidy && cd ..
cd demo-app      && go get go.temporal.io/api && go mod tidy && cd ..
```

> **Note:** `go.temporal.io/api` is a transitive dependency of the SDK but
> needs to be explicit in `demo-app/go.mod` because `metrics.go` imports
> `go.temporal.io/api/workflowservice/v1` directly for `CountWorkflowExecutionsRequest`.

Then sync the workspace:

```bash
go work sync
```

### 2. Start the Temporal dev server

In a dedicated terminal — leave this running:

```bash
temporal server start-dev
```

This starts a local Temporal cluster at `localhost:7233` with the Web UI at
[http://localhost:8233](http://localhost:8233). No auth, no TLS — perfect for
local dev.

### 3. Start a local worker

The Lambda worker uses `lambdaworker.RunWorker`, which is designed for Lambda
invocations and exits after each task batch — not ideal for local iteration.
Instead, run the long-polling local worker from `demo-app/localworker/`:

```bash
cd demo-app
go run ./localworker/main.go
```

Worker concurrency is configured via environment variables loaded from
`demo-app/localworker/.env`. The defaults are intentionally low (5 concurrent
activities, 5 concurrent workflow tasks) to simulate the capacity of a single
Lambda invocation — this makes backlog depth and sync match rate pressure
visible with a realistic seed count. Without this constraint the Go SDK default
of 1000 slots drains tasks instantly and nothing appears on the dashboard.

To override for a particular run without editing the file:

```bash
WORKER_MAX_CONCURRENT_ACTIVITIES=2 go run ./localworker/main.go
```

### 4. Start the demo app

In another terminal:

```bash
cd demo-app
LAMBDA_FUNCTION_NAME=local go run .
```

`LAMBDA_FUNCTION_NAME` is required by the metrics handler. Setting it to any
non-empty string (e.g. `local`) is fine for local dev — CloudWatch calls will
no-op gracefully when no AWS credentials are present, and the Lambda concurrency
stat will show `0`.

The demo app will be available at [http://localhost:8080](http://localhost:8080).

### 5. Try it out

Open [http://localhost:8080](http://localhost:8080), enter a name, and click
**Start workflow**. You should see:

- The running workflow count increment
- The activity feed populate with your submission
- The task queue backlog spike and the sync match rate drop
- The Temporal Web UI at [http://localhost:8233](http://localhost:8233) show
  the workflow execution in real time

To trigger a burst that saturates the worker and makes the scaling visuals
dramatic, use the presenter mode burst button (see [Presenter mode](#presenter-mode)
below) or hit the seed endpoint directly:

```bash
curl -X POST http://localhost:8080/api/seed?count=30
```

### Local environment summary

| Terminal | Command | Purpose |
|---|---|---|
| 1 | `temporal server start-dev` | Local Temporal cluster + Web UI |
| 2 | `cd demo-app && go run ./localworker/main.go` | Long-polling worker |
| 3 | `cd demo-app && LAMBDA_FUNCTION_NAME=local go run .` | Demo app server |

---

## Presenter mode

The UI has a hidden presenter panel for triggering workflow bursts during the
live demo. It is not visible to the audience by default.

**Activate** by either:
- Adding `?presenter=1` to the URL before screen sharing
- Pressing `Ctrl+Shift+P` at any time to toggle it on/off

The panel lets you fire N workflows simultaneously (default 30, max 200) to
prime the scaling visuals before or during the audience participation moment.

---

## Deploying to AWS + EKS

See the per-component READMEs (coming soon) for full deployment instructions.
High-level steps:

1. **Lambda worker** — run `./lambda-worker/mk-iam-role.sh` to create the IAM
   role Temporal Cloud assumes, then `make deploy` to build and push the
   function code.
2. **Temporal Cloud** — configure the serverless worker deployment version in
   your namespace settings, pointing at the IAM role ARN and Lambda function ARN.
3. **Demo app** — build and push the Docker image, then apply the k8s manifests:
   ```bash
   docker build -t <your-ecr-repo>/demo-app:latest -f demo-app/Dockerfile .
   docker push <your-ecr-repo>/demo-app:latest
   kubectl apply -f demo-app/k8s/
   ```

---

## Configuration reference

### Worker concurrency environment variables

Both the local worker and the Lambda worker read the same environment variables
from `shared/workerconfig`. The startup log always prints the values in use.

| Variable | Default | Description |
|---|---|---|
| `WORKER_MAX_CONCURRENT_ACTIVITIES` | `5` | Max activity tasks processed concurrently per worker instance |
| `WORKER_MAX_CONCURRENT_WORKFLOWS` | `5` | Max workflow tasks processed concurrently per worker instance |

**Local worker** — values are loaded from `demo-app/localworker/.env` (or from
the shell environment, which takes precedence). Edit `.env` to change the
defaults for all local dev runs.

**Lambda worker** — the `lambda-worker/.env` file documents these variables but
is **not** bundled into the Lambda zip by `deploy-lambda.sh`. Set them as
Lambda function environment variables in the AWS console, AWS CLI, or Terraform:

```bash
aws lambda update-function-configuration \
  --function-name serverless-demo-worker \
  --environment "Variables={WORKER_MAX_CONCURRENT_ACTIVITIES=5,WORKER_MAX_CONCURRENT_WORKFLOWS=5}"
```

### Demo app environment variables

| Variable | Required | Description |
|---|---|---|
| `LAMBDA_FUNCTION_NAME` | Yes | Lambda function name for CloudWatch metrics lookup |
| `TEMPORAL_ADDRESS` | No | Temporal server address (default: `localhost:7233`) |
| `TEMPORAL_NAMESPACE` | No | Temporal namespace (default: `default`) |
| `TEMPORAL_TLS_CERT` | No | Path to mTLS client cert (Temporal Cloud) |
| `TEMPORAL_TLS_KEY` | No | Path to mTLS client key (Temporal Cloud) |

### Tuning the demo

**Worker concurrency** is the primary lever for how quickly backlog builds. With
`WORKER_MAX_CONCURRENT_ACTIVITIES=5` and each `SimulateWork` activity sleeping
for 12 seconds, the worker saturates with just 5 concurrent workflows. A seed
burst of 30 immediately queues 25 activity tasks, driving the sync match rate
down and making the scaling signal visible.

**Activity sleep durations** control how long each workflow holds worker
capacity. Edit the constants in `shared/activities/demo_activities.go`:

```go
const (
    WorkDuration1 = 12 * time.Second
    WorkDuration2 = 12 * time.Second
    WorkDuration3 = 12 * time.Second
)
```

Each workflow chains all three activities sequentially (~36 seconds of total
worker occupancy). Increasing durations makes the scaling visuals last longer;
decreasing them speeds up the drain. The `StartToCloseTimeout` in
`shared/workflows/demo_workflow.go` must always exceed the sum of all three.
