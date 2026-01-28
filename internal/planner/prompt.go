package planner

import "fmt"

const plannerPrompt = `You are a task planner for a software development project.

Read the Product Requirements Document (PRD) below and extract a list of discrete implementation tasks.

## Requirements for tasks:
- Each task should be a single, well-defined unit of work
- Tasks should be implementable independently (though they may build on each other)
- Tasks should be concrete and actionable
- Include enough detail that a developer can understand what to implement
- Order tasks logically (foundational work first, features that depend on it later)

## Output format:
Output ONLY a JSON array of task objects. Each object must have:
- "title": A brief, descriptive title (under 60 characters)
- "description": Detailed implementation instructions
- "blocked_by": Array of 0-based indices of tasks that must complete first (e.g., [0, 1] means this task depends on tasks 0 and 1)

Do not include any other text, markdown formatting, or explanation.

Example output:
[{"title":"Create auth endpoint","description":"Create user authentication endpoint with JWT tokens","blocked_by":[]},{"title":"Add password hashing","description":"Add password hashing with bcrypt","blocked_by":[0]},{"title":"Create login form","description":"Create login form component with validation","blocked_by":[0,1]}]
%s
## PRD:
%s
`

func BuildPlannerPrompt(prdContent, plannerInstructions string) string {
	instructionsSection := ""
	if plannerInstructions != "" {
		instructionsSection = fmt.Sprintf("\n## Additional Instructions:\n%s\n", plannerInstructions)
	}
	return fmt.Sprintf(plannerPrompt, instructionsSection, prdContent)
}
