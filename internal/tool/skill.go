package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bobbyjohnstx/tinycode/internal/skill"
)

type skillArgs struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments,omitempty"`
}

// SkillTool returns a tool that executes a skill by name.
// It discovers skills, reads the SKILL.md content, substitutes positional
// parameters ($1, $2, $ARGUMENTS), and returns the result.
func SkillTool(configDir, projectDir string, skillPaths ...string) *Def {
	var paths []string
	if len(skillPaths) > 0 {
		paths = skillPaths
	}
	return &Def{
		ID:          "skill",
		Description: "Execute a skill by name. Skills are reusable prompt templates discovered from user and project config directories.",
		Permission:  "read",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "The name of the skill to execute",
				},
				"arguments": map[string]any{
					"type":        "string",
					"description": "Space-separated arguments to pass to the skill ($1, $2, etc.)",
				},
			},
			"required": []string{"name"},
		},
		Execute: func(ctx context.Context, tc *Context, rawArgs json.RawMessage) (*ExecuteResult, error) {
			return executeSkill(ctx, tc, rawArgs, configDir, projectDir, paths)
		},
	}
}

func executeSkill(_ context.Context, _ *Context, rawArgs json.RawMessage, configDir, projectDir string, skillPaths []string) (*ExecuteResult, error) {
	var args skillArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return &ExecuteResult{Output: fmt.Sprintf("Invalid arguments: %v", err), IsError: true}, nil
	}

	if args.Name == "" {
		return &ExecuteResult{Output: "skill name is required", IsError: true}, nil
	}

	skills := skill.DiscoverWithPaths(configDir, projectDir, skillPaths)

	var found *skill.Skill
	for i, s := range skills {
		if s.Name == args.Name || s.ID == args.Name {
			found = &skills[i]
			break
		}
	}

	if found == nil {
		var available []string
		for _, s := range skills {
			entry := s.Name
			if s.Description != "" {
				entry += " - " + s.Description
			}
			available = append(available, entry)
		}
		msg := fmt.Sprintf("Skill %q not found.", args.Name)
		if len(available) > 0 {
			msg += "\nAvailable skills:\n- " + strings.Join(available, "\n- ")
		} else {
			msg += "\nNo skills are available."
		}
		return &ExecuteResult{Output: msg, IsError: true}, nil
	}

	content, readErr := skill.LoadContent(*found)
	if readErr != nil {
		return &ExecuteResult{Output: fmt.Sprintf("Failed to read skill file: %v", readErr), IsError: true}, nil
	}

	result := skill.SubstituteParams(content, args.Arguments)
	return &ExecuteResult{Output: result}, nil
}

// substituteSkillParams is a thin wrapper kept for existing tests.
func substituteSkillParams(content, arguments string) string {
	return skill.SubstituteParams(content, arguments)
}
