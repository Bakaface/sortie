package action

import (
	"errors"
	"fmt"
)

type StopArgs struct {
	ID int64
}

func (a StopArgs) Validate() error {
	if a.ID <= 0 {
		return errors.New("task id is required")
	}
	return nil
}

func RunStop(ctx Ctx, args StopArgs) (Result, error) {
	if err := args.Validate(); err != nil {
		return Result{}, err
	}
	task, err := ctx.Client.StopTask(args.ID)
	if err != nil {
		return Result{}, fmt.Errorf("stop task %d: %w", args.ID, err)
	}
	return Result{
		Message: fmt.Sprintf("Stopped task %d", args.ID),
		Task:    task,
	}, nil
}

func init() {
	Registry["stop"] = Action{
		ID:   "stop",
		Help: "Stop a running task",
		Run: func(ctx Ctx, a Args) (Result, error) {
			return RunStop(ctx, a.(StopArgs))
		},
		Parse: func(raw string) (Args, error) {
			id, err := parseInt64(raw)
			if err != nil {
				return nil, err
			}
			return StopArgs{ID: id}, nil
		},
	}
}
