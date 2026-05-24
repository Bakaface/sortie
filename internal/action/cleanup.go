package action

import (
	"errors"
	"fmt"
)

type CleanupArgs struct {
	TaskID int64 // 0 == all completed/failed tasks
}

func (a CleanupArgs) Validate() error {
	if a.TaskID < 0 {
		return errors.New("task id must be >= 0")
	}
	return nil
}

func RunCleanup(ctx Ctx, args CleanupArgs) (Result, error) {
	if err := args.Validate(); err != nil {
		return Result{}, err
	}
	count, tasks, err := ctx.Client.Cleanup(args.TaskID)
	if err != nil {
		return Result{}, fmt.Errorf("cleanup: %w", err)
	}
	var msg string
	switch {
	case args.TaskID != 0 && count == 0:
		msg = fmt.Sprintf("Nothing to clean up for task #%d", args.TaskID)
	case args.TaskID != 0:
		msg = fmt.Sprintf("Cleaned up task #%d", args.TaskID)
	case count == 0:
		msg = "Nothing to clean up"
	default:
		msg = fmt.Sprintf("Cleaned up %d task(s)", count)
	}
	return Result{Message: msg, Count: count, Tasks: tasks}, nil
}

func init() {
	Registry["cleanup"] = Action{
		ID:   "cleanup",
		Help: "Remove worktrees for completed/failed tasks",
		Run: func(ctx Ctx, a Args) (Result, error) {
			return RunCleanup(ctx, a.(CleanupArgs))
		},
		Parse: func(raw string) (Args, error) {
			if raw == "" {
				return CleanupArgs{}, nil
			}
			id, err := parseInt64(raw)
			if err != nil {
				return nil, err
			}
			return CleanupArgs{TaskID: id}, nil
		},
	}
}
