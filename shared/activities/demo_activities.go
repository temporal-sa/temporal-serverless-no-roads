package activities

import (
	"context"
	"fmt"
	"time"

	"go.temporal.io/sdk/activity"

	"github.com/temporalio/temporal-serverless-no-roads/shared/workflows"
)

// Activities holds any dependencies needed by activity implementations.
type Activities struct{}

// ProcessSubmission is the first activity — fast validation and greeting
// construction. Completes quickly by design.
func (a *Activities) ProcessSubmission(ctx context.Context, input workflows.DemoInput) (string, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("ProcessSubmission activity started", "name", input.Name)

	result := fmt.Sprintf("Hello from Lambda, %s! Your workflow ran successfully.", input.Name)
	return result, nil
}

// Each of the three SimulateWork activities below holds a worker slot for its
// duration. Chaining three of them means each workflow occupies worker capacity
// for ~36 seconds total, which causes activity tasks to queue up behind busy
// workers under load — making backlog depth and sync match rate visible on the
// dashboard and triggering Temporal to invoke additional Lambda instances.
//
// Unlike workflow.Sleep (which releases the worker immediately), sleeping
// inside an activity holds the worker slot for the full duration — exactly
// what we need to demonstrate scaling under load.
//
// Tune individual durations by adjusting the constants below.
const (
	WorkDuration1 = 12 * time.Second
	WorkDuration2 = 12 * time.Second
	WorkDuration3 = 12 * time.Second
)

func (a *Activities) SimulateWork1(ctx context.Context, input workflows.DemoInput) (string, error) {
	return simulateSleep(ctx, input.Name, "SimulateWork1", WorkDuration1)
}

func (a *Activities) SimulateWork2(ctx context.Context, input workflows.DemoInput) (string, error) {
	return simulateSleep(ctx, input.Name, "SimulateWork2", WorkDuration2)
}

func (a *Activities) SimulateWork3(ctx context.Context, input workflows.DemoInput) (string, error) {
	return simulateSleep(ctx, input.Name, "SimulateWork3", WorkDuration3)
}

// simulateSleep is the shared implementation for the three work activities.
func simulateSleep(ctx context.Context, name, activityName string, d time.Duration) (string, error) {
	logger := activity.GetLogger(ctx)
	logger.Info(activityName+" started", "name", name, "duration", d)
	select {
	case <-time.After(d):
		logger.Info(activityName+" completed", "name", name)
		return fmt.Sprintf("%s complete for %s", activityName, name), nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}
