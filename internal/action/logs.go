package action

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

type LogsArgs struct {
	ID   int64
	Tail int
}

func (a LogsArgs) Validate() error {
	if a.ID <= 0 {
		return errors.New("task id is required")
	}
	if a.Tail < 0 {
		return errors.New("tail must be >= 0")
	}
	return nil
}

// RunLogs returns a one-shot snapshot of a task's log file. The CLI's
// `--follow` mode keeps its own streaming goroutine and never calls into
// here.
func RunLogs(ctx Ctx, args LogsArgs) (Result, error) {
	if err := args.Validate(); err != nil {
		return Result{}, err
	}
	lines, _, err := ctx.Client.GetLogs(args.ID, args.Tail, 0)
	if err != nil {
		return Result{}, fmt.Errorf("get logs for task %d: %w", args.ID, err)
	}
	if len(lines) == 0 {
		return Result{Message: "No logs available"}, nil
	}
	return Result{Message: strings.Join(lines, "\n")}, nil
}

func init() {
	Registry["logs"] = Action{
		ID:   "logs",
		Help: "Show recent logs for a task",
		Run: func(ctx Ctx, a Args) (Result, error) {
			return RunLogs(ctx, a.(LogsArgs))
		},
		Parse: func(raw string) (Args, error) {
			parts := parseFields(raw)
			if len(parts) == 0 {
				return nil, errors.New("expected: logs <task-id> [tail]")
			}
			id, err := parseInt64(parts[0])
			if err != nil {
				return nil, err
			}
			args := LogsArgs{ID: id}
			if len(parts) > 1 {
				n, err := strconv.Atoi(parts[1])
				if err != nil || n < 0 {
					return nil, fmt.Errorf("invalid tail %q", parts[1])
				}
				args.Tail = n
			}
			return args, nil
		},
	}
}
