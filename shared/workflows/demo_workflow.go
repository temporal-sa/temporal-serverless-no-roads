package workflows

import (
	"time"

	"go.temporal.io/sdk/workflow"
)

// DemoInput is the input to the DemoWorkflow — just the submitter's name.
type DemoInput struct {
	Name string `json:"name"`
}

// DemoOutput is returned when the workflow completes.
type DemoOutput struct {
	Message string `json:"message"`
}

// DemoWorkflow is the workflow that gets triggered by each audience submission.
//
// It runs four activities in sequence:
//   - ProcessSubmission: fast, constructs the greeting
//   - SimulateWork1/2/3: each holds a worker slot for ~12 seconds (~36s total)
//
// Chaining three sleep activities means each workflow occupies worker capacity
// long enough that concurrent submissions cause activity tasks to visibly queue
// up behind busy workers, driving down sync match rate and triggering Temporal
// to invoke additional Lambda instances.
func DemoWorkflow(ctx workflow.Context, input DemoInput) (DemoOutput, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("DemoWorkflow started", "name", input.Name)

	ao := workflow.ActivityOptions{
		// Must exceed the sum of all three WorkDuration constants.
		StartToCloseTimeout: 60 * time.Second,
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	// Step 1: fast processing activity
	var result string
	if err := workflow.ExecuteActivity(ctx, "ProcessSubmission", input).Get(ctx, &result); err != nil {
		return DemoOutput{}, err
	}

	// Steps 2-4: chained sleep activities — each holds a worker slot, driving
	// backlog depth and sync match rate pressure under load.
	var workResult string
	if err := workflow.ExecuteActivity(ctx, "SimulateWork1", input).Get(ctx, &workResult); err != nil {
		return DemoOutput{}, err
	}
	if err := workflow.ExecuteActivity(ctx, "SimulateWork2", input).Get(ctx, &workResult); err != nil {
		return DemoOutput{}, err
	}
	if err := workflow.ExecuteActivity(ctx, "SimulateWork3", input).Get(ctx, &workResult); err != nil {
		return DemoOutput{}, err
	}

	logger.Info("DemoWorkflow completed", "name", input.Name)
	return DemoOutput{Message: result}, nil
}
