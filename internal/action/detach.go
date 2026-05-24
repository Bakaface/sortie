package action

import (
	"errors"
	"fmt"
)

type DetachArgs struct {
	ID int64
}

func (a DetachArgs) Validate() error {
	if a.ID <= 0 {
		return errors.New("task id is required")
	}
	return nil
}

func RunDetach(ctx Ctx, args DetachArgs) (Result, error) {
	if err := args.Validate(); err != nil {
		return Result{}, err
	}
	task, err := ctx.Client.DetachBranch(args.ID)
	if err != nil {
		return Result{}, fmt.Errorf("detach task %d: %w", args.ID, err)
	}
	return Result{
		Message: fmt.Sprintf("Branch detached from task #%d worktree", args.ID),
		Task:    task,
	}, nil
}

func init() {
	Registry["detach"] = Action{
		ID:   "detach",
		Help: "Detach worktree branch so it can be checked out elsewhere",
		Run: func(ctx Ctx, a Args) (Result, error) {
			return RunDetach(ctx, a.(DetachArgs))
		},
		Parse: func(raw string) (Args, error) {
			id, err := parseInt64(raw)
			if err != nil {
				return nil, err
			}
			return DetachArgs{ID: id}, nil
		},
	}
}
