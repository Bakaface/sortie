package action

import (
	"errors"
	"fmt"
	"strings"
)

type ContinueArgs struct {
	ID       int64
	Workflow string
	Prompt   string
}

func (a ContinueArgs) Validate() error {
	if a.ID <= 0 {
		return errors.New("task id is required")
	}
	return nil
}

func RunContinue(ctx Ctx, args ContinueArgs) (Result, error) {
	if err := args.Validate(); err != nil {
		return Result{}, err
	}
	task, err := ctx.Client.ContinueTask(args.ID, args.Workflow, args.Prompt)
	if err != nil {
		return Result{}, fmt.Errorf("continue task %d: %w", args.ID, err)
	}
	msg := fmt.Sprintf("Continue session started for task #%d", args.ID)
	if args.Workflow != "" {
		msg = fmt.Sprintf("Task #%d continuing with workflow %q", args.ID, args.Workflow)
	}
	return Result{Message: msg, Task: task}, nil
}

func init() {
	Registry["continue"] = Action{
		ID:   "continue",
		Help: "Continue a task (awaiting-approval, completed, failed, or tmux)",
		Run: func(ctx Ctx, a Args) (Result, error) {
			return RunContinue(ctx, a.(ContinueArgs))
		},
		Parse: func(raw string) (Args, error) {
			parts := parseFields(raw)
			if len(parts) == 0 {
				return nil, errors.New("expected: continue <task-id> [workflow]")
			}
			id, err := parseInt64(parts[0])
			if err != nil {
				return nil, err
			}
			args := ContinueArgs{ID: id}
			if len(parts) > 1 {
				args.Workflow = parts[1]
			}
			if len(parts) > 2 {
				args.Prompt = strings.Join(parts[2:], " ")
			}
			return args, nil
		},
	}
}
