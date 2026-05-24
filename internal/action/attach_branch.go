package action

import (
	"errors"
	"fmt"
)

type AttachBranchArgs struct {
	ID int64
}

func (a AttachBranchArgs) Validate() error {
	if a.ID <= 0 {
		return errors.New("task id is required")
	}
	return nil
}

func RunAttachBranch(ctx Ctx, args AttachBranchArgs) (Result, error) {
	if err := args.Validate(); err != nil {
		return Result{}, err
	}
	task, err := ctx.Client.AttachBranch(args.ID)
	if err != nil {
		return Result{}, fmt.Errorf("attach branch on task %d: %w", args.ID, err)
	}
	return Result{
		Message: fmt.Sprintf("Branch reattached to task #%d worktree", args.ID),
		Task:    task,
	}, nil
}

func init() {
	Registry["attach-branch"] = Action{
		ID:   "attach-branch",
		Help: "Reattach branch to worktree after detach",
		Run: func(ctx Ctx, a Args) (Result, error) {
			return RunAttachBranch(ctx, a.(AttachBranchArgs))
		},
		Parse: func(raw string) (Args, error) {
			id, err := parseInt64(raw)
			if err != nil {
				return nil, err
			}
			return AttachBranchArgs{ID: id}, nil
		},
	}
}
