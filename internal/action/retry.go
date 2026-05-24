package action

import (
	"errors"
	"fmt"
)

type RetryArgs struct {
	ID       int64
	FromStep string
}

func (a RetryArgs) Validate() error {
	if a.ID <= 0 {
		return errors.New("task id is required")
	}
	return nil
}

func RunRetry(ctx Ctx, args RetryArgs) (Result, error) {
	if err := args.Validate(); err != nil {
		return Result{}, err
	}
	task, err := ctx.Client.RetryTask(args.ID, args.FromStep)
	if err != nil {
		return Result{}, fmt.Errorf("retry task %d: %w", args.ID, err)
	}
	msg := fmt.Sprintf("Task #%d reset for retry", args.ID)
	if args.FromStep != "" {
		msg = fmt.Sprintf("Task #%d reset for retry from step %q", args.ID, args.FromStep)
	}
	return Result{
		Message: msg,
		Task:    task,
	}, nil
}

func init() {
	Registry["retry"] = Action{
		ID:   "retry",
		Help: "Retry a failed task",
		Run: func(ctx Ctx, a Args) (Result, error) {
			return RunRetry(ctx, a.(RetryArgs))
		},
		Parse: func(raw string) (Args, error) {
			id, err := parseInt64(raw)
			if err != nil {
				return nil, err
			}
			return RetryArgs{ID: id}, nil
		},
	}
}
