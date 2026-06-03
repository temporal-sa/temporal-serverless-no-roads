package main

import (
	"context"
	"log"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"go.temporal.io/sdk/contrib/aws/lambdaworker"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"

	"github.com/temporalio/temporal-serverless-no-roads/shared/activities"
	"github.com/temporalio/temporal-serverless-no-roads/shared/taskqueue"
	"github.com/temporalio/temporal-serverless-no-roads/shared/workerconfig"
	"github.com/temporalio/temporal-serverless-no-roads/shared/workflows"
)

func main() {
	cfg := workerconfig.Load()

	// Resolve the Temporal API key. Prefer TEMPORAL_API_KEY if already set
	// (e.g. set directly as a Lambda environment variable for simplicity).
	// If not set, fall back to fetching from Secrets Manager using the path
	// in TEMPORAL_API_KEY_SECRET_ARN.
	if os.Getenv("TEMPORAL_API_KEY") == "" {
		secretArn := os.Getenv("TEMPORAL_API_KEY_SECRET_ARN")
		if secretArn == "" {
			log.Fatal("one of TEMPORAL_API_KEY or TEMPORAL_API_KEY_SECRET_ARN must be set")
		}
		apiKey := mustFetchSecret(secretArn)
		os.Setenv("TEMPORAL_API_KEY", apiKey)
	}

	// TEMPORAL_ADDRESS and TEMPORAL_NAMESPACE are read automatically by the
	// SDK from environment variables — no temporal.toml required.
	// Set them as Lambda function environment variables:
	//   TEMPORAL_ADDRESS   = <your-namespace>.<account-id>.tmprl.cloud:7233
	//   TEMPORAL_NAMESPACE = <your-namespace>.<account-id>
	//
	// TLS is enabled automatically by the SDK when TEMPORAL_API_KEY is set.

	lambdaworker.RunWorker(
		worker.WorkerDeploymentVersion{
			DeploymentName: taskqueue.DemoTaskQueue,
			BuildID:        taskqueue.DemoBuildID,
		},
		func(opts *lambdaworker.Options) error {
			opts.TaskQueue = taskqueue.DemoTaskQueue
			opts.WorkerOptions.MaxConcurrentActivityExecutionSize = cfg.MaxConcurrentActivityExecutionSize
			opts.WorkerOptions.MaxConcurrentWorkflowTaskExecutionSize = cfg.MaxConcurrentWorkflowTaskExecutionSize
			// Disable eager activity dispatch so activity tasks flow through the
			// task queue rather than being handed directly to the completing worker.
			// Without this, TasksAddRate stays zero because tasks never touch the
			// queue — making backlog and sync match rate metrics invisible.
			opts.WorkerOptions.DisableEagerActivities = true

			// Opt the worker into deployment-based versioning. Without
			// UseVersioning: true, Temporal won't route versioned tasks here.
			opts.WorkerOptions.DeploymentOptions = worker.DeploymentOptions{
				UseVersioning:             true,
				Version: worker.WorkerDeploymentVersion{
					DeploymentName: "serverless-webinar",
					BuildID:        "1.0.0",
				},
				DefaultVersioningBehavior: workflow.VersioningBehaviorPinned,
			}

			// Build client options from env vars. TEMPORAL_ADDRESS and
			// TEMPORAL_NAMESPACE are read automatically by the SDK.
			// TEMPORAL_API_KEY (already resolved above) is picked up by
			// BuildClientOptions; the SDK auto-enables TLS when Credentials are set.
			opts.ClientOptions = workerconfig.BuildClientOptions()

			opts.RegisterWorkflow(workflows.DemoWorkflow)
			opts.RegisterActivity(&activities.Activities{})

			return nil
		},
	)
}

// mustFetchSecret retrieves a plaintext secret value from AWS Secrets Manager.
// Logs a fatal error and exits if the fetch fails — Lambda cold start must
// succeed fully or not at all.
func mustFetchSecret(secretArn string) string {
	ctx := context.Background()

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		log.Fatalf("lambda-worker: failed to load AWS config: %v", err)
	}

	sm := secretsmanager.NewFromConfig(awsCfg)
	out, err := sm.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(secretArn),
	})
	if err != nil {
		log.Fatalf("lambda-worker: failed to fetch secret %s: %v", secretArn, err)
	}

	if out.SecretString == nil {
		log.Fatalf("lambda-worker: secret %s has no string value", secretArn)
	}
	return *out.SecretString
}
