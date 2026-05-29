# temporal-serverless-no-roads

> *We Don't Need Workers Where We're Going*

A live audience-participation demo for Temporal's Serverless Workers feature. Attendees submit their name via a web UI to trigger real workflow executions, and watch Lambda invocations, task queue backlog, and workflow counts update in real time.

## Repo structure

```
temporal-serverless-no-roads/
├── shared/                    # Shared Go module — workflows, activities, task queue, worker config
│   ├── activities/            # ProcessSubmission + SimulateWork1/2/3 activity implementations
│   ├── taskqueue/             # Shared task queue name constant
│   ├── workerconfig/          # Reads worker concurrency from environment variables
│   └── workflows/             # DemoWorkflow definition
├── lambda-worker/             # Deployable 1: Lambda worker (Go)
│   ├── cfn/
│   │   └── execution-role.yaml    # CloudFormation: Lambda execution role (CloudWatch + Secrets Manager)
│   ├── deploy-lambda.sh       # Build, package, create-or-update Lambda function
│   ├── Makefile               # Orchestrates cfn-execution-role → deploy
│   ├── temporal.toml          # Temporal Cloud connection config (update before deploying)
│   └── .env                   # Documents Lambda environment variables (not bundled in zip)
├── demo-app/                  # Deployable 2: HTTP server — UI + API (Go)
│   ├── api/                   # /api/submit, /api/metrics, /api/seed handlers
│   ├── cache/                 # Short-TTL metrics cache
│   ├── frontend/              # Embedded HTML UI (served at /)
│   ├── localworker/           # Long-polling worker for local dev (not deployed to Lambda)
│   │   └── .env               # Worker concurrency settings for local dev
│   ├── middleware/            # Per-IP rate limiter
│   └── k8s/                   # Kubernetes manifests for EKS deployment
├── go.work                    # Go workspace — ties all three modules together
└── README.md
```

> Note: `cfn/temporal-invoke-role.yaml` has been removed. The Temporal Cloud UI
> provides its own CloudFormation template for the invocation role as part of
> the worker deployment creation flow (Step 6).

---

## Running locally

Local dev uses the Temporal CLI dev server in place of Temporal Cloud, and a
standard long-polling worker in place of the Lambda worker. No AWS account or
Temporal Cloud namespace required.

### Prerequisites

- [Go 1.22+](https://go.dev/dl/)
- [Temporal CLI](https://docs.temporal.io/cli#installation)

```bash
# macOS
brew install temporal

# Linux — see https://temporal.download/cli/archive/latest?platform=linux&arch=amd64
```

### 1. Resolve dependencies

Run `go mod tidy` in each module. The workspace root has no `go.mod`, so this
must be done per-module:

```bash
cd shared        && go mod tidy && cd ..
cd lambda-worker && go mod tidy && cd ..
cd demo-app      && go get go.temporal.io/api && go mod tidy && cd ..
go work sync
```

> `go.temporal.io/api` needs an explicit entry in `demo-app/go.mod` because
> `metrics.go` imports `go.temporal.io/api/workflowservice/v1` directly for
> `CountWorkflowExecutionsRequest`.

### 2. Start the Temporal dev server

```bash
temporal server start-dev
```

Starts a local Temporal cluster at `localhost:7233`. Web UI at
[http://localhost:8233](http://localhost:8233). No auth, no TLS.

### 3. Start a local worker

The Lambda worker uses `lambdaworker.RunWorker`, which exits after each task
batch and is not suited for local iteration. Use the long-polling local worker
instead:

```bash
cd demo-app
go run ./localworker/main.go
```

Concurrency is configured via `demo-app/localworker/.env`. The defaults (5
concurrent activities, 5 workflow tasks) are intentionally low to simulate
single-Lambda-invocation capacity, making backlog depth and sync match rate
pressure visible with a realistic seed count. To override for one run:

```bash
WORKER_MAX_CONCURRENT_ACTIVITIES=2 go run ./localworker/main.go
```

### 4. Start the demo app

```bash
cd demo-app
LAMBDA_FUNCTION_NAME=local go run .
```

`LAMBDA_FUNCTION_NAME` is required by the metrics handler — set it to any
non-empty string locally. CloudWatch calls degrade gracefully to `0` without
AWS credentials.

The demo app is available at [http://localhost:8080](http://localhost:8080).

### 5. Try it out

Open [http://localhost:8080](http://localhost:8080), enter a name, click **Start
workflow**, and watch the dashboard update. To saturate the worker and produce
visible backlog and sync match rate pressure, fire a seed burst:

```bash
curl -X POST http://localhost:8080/api/seed?count=30
```

Or activate presenter mode in the UI (`?presenter=1` in the URL, or
`Ctrl+Shift+P`) and use the Fire Burst button.

### Local environment summary

| Terminal | Command | Purpose |
|---|---|---|
| 1 | `temporal server start-dev` | Local Temporal cluster + Web UI |
| 2 | `cd demo-app && go run ./localworker/main.go` | Long-polling worker |
| 3 | `cd demo-app && LAMBDA_FUNCTION_NAME=local go run .` | Demo app server |

---

## Deploying the Lambda worker to AWS

This section covers the complete end-to-end process for deploying the Lambda
worker to the SA AWS account and connecting it to Temporal Cloud.

### Prerequisites

- A Temporal Cloud namespace with mTLS client cert and key. Download them from
  the Temporal Cloud UI under your namespace → **Certificates**.

- Temporal CLI installed (`brew install temporal`).

### Step 1 — Store Temporal credentials in Secrets Manager

Store your mTLS cert and key so the Lambda can read them at startup without
bundling secrets in the deployment zip.

### Step 2 — Update `temporal.toml`

Edit `lambda-worker/temporal.toml` with your Temporal Cloud namespace address:

```toml
[profile.default]
address   = "<your-namespace>.<account-id>.tmprl.cloud:7233"
namespace = "<your-namespace>.<account-id>"

[profile.default.tls]
client-cert = "/tmp/client.pem"
client-key  = "/tmp/client.key"
```

### Step 3 — Deploy the Lambda execution role (CloudFormation)

Creates the IAM execution role that the Lambda function assumes at runtime. This
role grants CloudWatch Logs write access and Secrets Manager read access for the
Temporal credentials stored in Step 1.

```bash
cd lambda-worker
make cfn-execution-role
```

Prints the role ARN when complete. Copy it — you need it in Step 4.

### Step 4 — Create the Lambda function and deploy worker code

On first run this creates the Lambda function; on subsequent runs it updates the
code only.

```bash
make deploy \
  EXECUTION_ROLE=arn:aws:iam::<your-aws-account-id>:role/serverless-webinar-worker-execution-role
```

This cross-compiles the Go binary for `linux/amd64`, zips it with
`temporal.toml`, creates (or updates) the Lambda function with a 600-second
timeout and 256 MB memory, and sets the worker concurrency environment variables.

Note the **Lambda ARN** printed in the output — you need it in Step 6.

### Step 5 — Set Lambda environment variables

Worker concurrency is controlled by environment variables. Update them on the
deployed function:

```bash
aws lambda update-function-configuration \
  --function-name serverless-webinar-worker \
  --environment "Variables={WORKER_MAX_CONCURRENT_ACTIVITIES=5,WORKER_MAX_CONCURRENT_WORKFLOWS=5}" \
  --region us-east-1 \
  --profile <your-profile>
```

See the [Configuration reference](#configuration-reference) section for tuning
guidance.

### Step 6 — Create the worker deployment in Temporal Cloud

This step is done entirely in the Temporal Cloud UI. It wires the Lambda ARN to
Temporal and creates the IAM role Temporal uses to invoke it.

1. Open your namespace in the [Temporal Cloud UI](https://cloud.temporal.io).
2. Navigate to **Workers → Serverless** and click **Create Worker Deployment**.
3. Fill in:
   - **Name**: `serverless-webinar`
   - **Build ID**: `1.0.0`
   - **Compute**: AWS Lambda
   - **Lambda ARN**: the ARN from Step 4
4. The right panel shows a CloudFormation template. Deploy it to create the IAM
   role that Temporal Cloud uses to invoke your Lambda:
   - Copy or download the template from the UI
   - Deploy it:
     ```bash
     aws cloudformation deploy \
       --stack-name temporal-webinar-invoke-role \
       --template-file <downloaded-template.yaml> \
       --capabilities CAPABILITY_NAMED_IAM \
       --region us-east-1 \
       --profile <your-profile>
     ```
   - Note the **IAM Role ARN** from the stack outputs
5. Back in the Temporal Cloud UI, fill in:
   - **IAM Role ARN**: the ARN from the CloudFormation stack above
   - **External ID**: pre-filled by the UI (matches the deployment name)
6. Optionally expand **Show Scaling and Limits** to configure min/max instances
   before submitting.
7. Click **Create**.

After creation, Temporal Cloud will invoke your Lambda automatically whenever
workflows are started on the `serverless-webinar` task queue.

### Step 7 — Verify

Start a test workflow to confirm end-to-end connectivity:

```bash
temporal workflow start \
  --type DemoWorkflow \
  --task-queue serverless-webinar \
  --input '{"name":"test"}' \
  --address <your-namespace>.<account-id>.tmprl.cloud:7233 \
  --namespace <your-namespace>.<account-id> \
  --tls-cert-path /path/to/client.pem \
  --tls-key-path /path/to/client.key
```

Check the Temporal Cloud UI for the running workflow and the AWS Lambda console
for an invocation. If it executes and completes, everything is wired up
correctly.

### Lambda deployment summary

| Step | What it does |
|---|---|
| 1 | Store mTLS certs in Secrets Manager |
| 2 | Update `temporal.toml` with namespace address |
| 3 | `make cfn-execution-role` — Lambda execution role |
| 4 | `make deploy EXECUTION_ROLE=...` — create Lambda + upload code |
| 5 | `aws lambda update-function-configuration` — set concurrency env vars |
| 6 | Temporal Cloud UI — deploy CFN template, create worker deployment |
| 7 | `temporal workflow start` — end-to-end smoke test |

### Redeploying after code changes

Once the Lambda function and IAM roles exist, redeploy is just:

```bash
cd lambda-worker
make deploy EXECUTION_ROLE=arn:aws:iam::<your-aws-account-id>:role/serverless-webinar-worker-execution-role
```

If you bump the `BuildID` in `lambda-worker/main.go`, open the Temporal Cloud UI
and create a new deployment version with the updated build ID, then set it as
current.

---

## Deploying the demo app to EKS

### Prerequisites

- Docker and `kubectl` configured for the SA EKS cluster
- An ECR repository for the demo app image

### 1. Build and push the Docker image

```bash
# Authenticate Docker to ECR
aws ecr get-login-password \
  --region us-east-1 \
  --profile <your-profile> \
  | docker login --username AWS --password-stdin \
    <your-aws-account-id>.dkr.ecr.us-east-1.amazonaws.com

# Build from the repo root (Dockerfile references ../shared)
docker build \
  -t <your-aws-account-id>.dkr.ecr.us-east-1.amazonaws.com/serverless-webinar-app:latest \
  -f demo-app/Dockerfile .

docker push <your-aws-account-id>.dkr.ecr.us-east-1.amazonaws.com/serverless-webinar-app:latest
```

### 2. Create the Temporal credentials Secret

```bash
kubectl create secret generic temporal-credentials \
  --from-literal=address='<your-namespace>.<account-id>.tmprl.cloud:7233' \
  --from-literal=namespace='<your-namespace>.<account-id>' \
  --from-file=tls-cert=/path/to/client.pem \
  --from-file=tls-key=/path/to/client.key
```

### 3. Create the IRSA role for CloudWatch access

The demo app polls CloudWatch for Lambda concurrency metrics. Create an IAM role
annotated for IRSA (IAM Roles for Service Accounts) that grants
`cloudwatch:GetMetricStatistics`:

```bash
# Replace <oidc-provider> with your EKS cluster's OIDC provider URL
aws iam create-role \
  --role-name demo-app-cloudwatch-role \
  --assume-role-policy-document '{
    "Version": "2012-10-17",
    "Statement": [{
      "Effect": "Allow",
      "Principal": { "Federated": "arn:aws:iam::<your-aws-account-id>:oidc-provider/<oidc-provider>" },
      "Action": "sts:AssumeRoleWithWebIdentity",
      "Condition": {
        "StringEquals": {
          "<oidc-provider>:sub": "system:serviceaccount:default:demo-app"
        }
      }
    }]
  }' \
  --profile <your-profile>

aws iam put-role-policy \
  --role-name demo-app-cloudwatch-role \
  --policy-name CloudWatchGetMetrics \
  --policy-document '{
    "Version": "2012-10-17",
    "Statement": [{
      "Effect": "Allow",
      "Action": "cloudwatch:GetMetricStatistics",
      "Resource": "*"
    }]
  }' \
  --profile <your-profile>
```

### 4. Update k8s manifests and apply

Update `demo-app/k8s/deployment.yaml` with your ECR image URI and Lambda
function name. Update `demo-app/k8s/service.yaml` with your IRSA role ARN.
Then apply:

```bash
kubectl apply -f demo-app/k8s/
```

The demo app will be accessible via the ALB Ingress URL printed by:

```bash
kubectl get ingress demo-app
```

---

## Presenter mode

The UI has a hidden presenter panel for triggering workflow bursts during the
live demo. Not visible to the audience by default.

**Activate** by either:
- Adding `?presenter=1` to the URL before screen sharing
- Pressing `Ctrl+Shift+P` at any time to toggle

The panel fires N workflows simultaneously (default 30, max 200) to prime the
scaling visuals before or during the audience participation moment.

---

## Configuration reference

### Worker concurrency environment variables

Both the local worker and the Lambda worker share the same variables via
`shared/workerconfig`. The startup log always prints the values in use.

| Variable | Default | Description |
|---|---|---|
| `WORKER_MAX_CONCURRENT_ACTIVITIES` | `5` | Max activity tasks per worker instance |
| `WORKER_MAX_CONCURRENT_WORKFLOWS` | `5` | Max workflow tasks per worker instance |

**Local worker** — loaded from `demo-app/localworker/.env`. Shell environment
takes precedence.

**Lambda worker** — set as Lambda function environment variables. The `.env`
file in `lambda-worker/` documents them but is not bundled in the deployment
zip. `make deploy` sets defaults at function creation; update with:

```bash
aws lambda update-function-configuration \
  --function-name serverless-webinar-worker \
  --environment "Variables={WORKER_MAX_CONCURRENT_ACTIVITIES=5,WORKER_MAX_CONCURRENT_WORKFLOWS=5}" \
  --region us-east-1 \
  --profile <your-profile>
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
for 12 seconds, the worker saturates at 5 concurrent workflows. A seed burst of
30 immediately queues 25 activity tasks, driving the sync match rate down.

**Activity sleep durations** control how long each workflow holds worker
capacity. Edit the constants in `shared/activities/demo_activities.go`:

```go
const (
    WorkDuration1 = 12 * time.Second
    WorkDuration2 = 12 * time.Second
    WorkDuration3 = 12 * time.Second
)
```

Each workflow chains all three sequentially (~36 seconds total worker
occupancy). `StartToCloseTimeout` in `shared/workflows/demo_workflow.go` must
always exceed the sum of all three.

**Lambda timeout** — the function is created with a 600-second timeout. This
must be longer than the longest single activity execution plus the graceful
shutdown window. The current activity chain (36 seconds) is well within this
limit.

**Lambda memory** — set to 256 MB at creation. Increase if you observe memory
pressure in CloudWatch Logs.
