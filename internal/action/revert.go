package action

import (
	"errors"
	"fmt"
)

type RevertArgs struct {
	ID int64
}

func (a RevertArgs) Validate() error {
	if a.ID <= 0 {
		return errors.New("task id is required")
	}
	return nil
}

func RunRevert(ctx Ctx, args RevertArgs) (Result, error) {
	if err := args.Validate(); err != nil {
		return Result{}, err
	}
	task, err := ctx.Client.RevertTask(args.ID)
	if err != nil {
		return Result{}, fmt.Errorf("revert task %d: %w", args.ID, err)
	}
	return Result{
		Message: fmt.Sprintf("Task #%d reverted", args.ID),
		Task:    task,
	}, nil
}

func init() {
	Registry["revert"] = Action{
		ID:   "revert",
		Help: "Revert all commits made by a task",
		Run: func(ctx Ctx, a Args) (Result, error) {
			return RunRevert(ctx, a.(RevertArgs))
		},
		Parse: func(raw string) (Args, error) {
			id, err := parseInt64(raw)
			if err != nil {
				return nil, err
			}
			return RevertArgs{ID: id}, nil
		},
	}
}
