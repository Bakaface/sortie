package action

import (
	"errors"
	"fmt"
)

type DeleteArgs struct {
	ID int64
}

func (a DeleteArgs) Validate() error {
	if a.ID <= 0 {
		return errors.New("task id is required")
	}
	return nil
}

func RunDelete(ctx Ctx, args DeleteArgs) (Result, error) {
	if err := args.Validate(); err != nil {
		return Result{}, err
	}
	if err := ctx.Client.DeleteTask(args.ID); err != nil {
		return Result{}, fmt.Errorf("delete task %d: %w", args.ID, err)
	}
	return Result{
		Message: fmt.Sprintf("Task #%d deleted", args.ID),
	}, nil
}

func init() {
	Registry["delete"] = Action{
		ID:   "delete",
		Help: "Delete a task and its worktree",
		Run: func(ctx Ctx, a Args) (Result, error) {
			return RunDelete(ctx, a.(DeleteArgs))
		},
		Parse: func(raw string) (Args, error) {
			id, err := parseInt64(raw)
			if err != nil {
				return nil, err
			}
			return DeleteArgs{ID: id}, nil
		},
	}
}
