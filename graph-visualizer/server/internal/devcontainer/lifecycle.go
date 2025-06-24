package devcontainer

import (
	"fmt"
	"strings"
)

// LifecycleCommand represents a parsed lifecycle command
type LifecycleCommand struct {
	// Type of command: "string", "array", or "object"
	Type string
	
	// For string commands
	Command string
	
	// For array commands
	Args []string
	
	// For object commands (parallel execution)
	Commands map[string]LifecycleCommand
}

// ParseLifecycleCommand parses a lifecycle command from the devcontainer spec
func ParseLifecycleCommand(cmd interface{}) (*LifecycleCommand, error) {
	if cmd == nil {
		return nil, nil
	}

	switch v := cmd.(type) {
	case string:
		return &LifecycleCommand{
			Type:    "string",
			Command: v,
		}, nil
		
	case []interface{}:
		args := make([]string, 0, len(v))
		for _, arg := range v {
			str, ok := arg.(string)
			if !ok {
				return nil, fmt.Errorf("array command contains non-string element: %T", arg)
			}
			args = append(args, str)
		}
		return &LifecycleCommand{
			Type: "array",
			Args: args,
		}, nil
		
	case map[string]interface{}:
		commands := make(map[string]LifecycleCommand)
		for name, subcmd := range v {
			parsed, err := ParseLifecycleCommand(subcmd)
			if err != nil {
				return nil, fmt.Errorf("failed to parse command %q: %w", name, err)
			}
			if parsed != nil {
				commands[name] = *parsed
			}
		}
		return &LifecycleCommand{
			Type:     "object",
			Commands: commands,
		}, nil
		
	default:
		return nil, fmt.Errorf("unsupported lifecycle command type: %T", cmd)
	}
}

// ToShellCommand converts a lifecycle command to a shell command string
func (lc *LifecycleCommand) ToShellCommand() string {
	if lc == nil {
		return ""
	}

	switch lc.Type {
	case "string":
		return lc.Command
		
	case "array":
		// Quote arguments that contain spaces
		quotedArgs := make([]string, len(lc.Args))
		for i, arg := range lc.Args {
			if strings.Contains(arg, " ") && !strings.HasPrefix(arg, `"`) {
				quotedArgs[i] = fmt.Sprintf(`"%s"`, arg)
			} else {
				quotedArgs[i] = arg
			}
		}
		return strings.Join(quotedArgs, " ")
		
	case "object":
		// For object commands, we'd need to handle parallel execution
		// For now, return a comment indicating multiple commands
		var cmds []string
		for name := range lc.Commands {
			cmds = append(cmds, name)
		}
		return fmt.Sprintf("# Multiple commands: %s", strings.Join(cmds, ", "))
		
	default:
		return ""
	}
}

// GetAllCommands returns all commands (for object type, flattens to array)
func (lc *LifecycleCommand) GetAllCommands() []string {
	if lc == nil {
		return nil
	}

	switch lc.Type {
	case "string":
		return []string{lc.Command}
		
	case "array":
		return []string{strings.Join(lc.Args, " ")}
		
	case "object":
		var commands []string
		for _, cmd := range lc.Commands {
			commands = append(commands, cmd.ToShellCommand())
		}
		return commands
		
	default:
		return nil
	}
}

// ProcessLifecycleCommands processes all lifecycle commands in a DevContainer
func ProcessLifecycleCommands(dc *DevContainer) map[string]*LifecycleCommand {
	if dc == nil {
		return nil
	}

	commands := make(map[string]*LifecycleCommand)

	// Parse each lifecycle command
	if cmd, err := ParseLifecycleCommand(dc.InitializeCommand); err == nil && cmd != nil {
		commands["initializeCommand"] = cmd
	}
	
	if cmd, err := ParseLifecycleCommand(dc.OnCreateCommand); err == nil && cmd != nil {
		commands["onCreateCommand"] = cmd
	}
	
	if cmd, err := ParseLifecycleCommand(dc.UpdateContentCommand); err == nil && cmd != nil {
		commands["updateContentCommand"] = cmd
	}
	
	if cmd, err := ParseLifecycleCommand(dc.PostCreateCommand); err == nil && cmd != nil {
		commands["postCreateCommand"] = cmd
	}
	
	if cmd, err := ParseLifecycleCommand(dc.PostStartCommand); err == nil && cmd != nil {
		commands["postStartCommand"] = cmd
	}
	
	if cmd, err := ParseLifecycleCommand(dc.PostAttachCommand); err == nil && cmd != nil {
		commands["postAttachCommand"] = cmd
	}

	return commands
}

// GetLifecycleScript generates a shell script for lifecycle commands
func GetLifecycleScript(dc *DevContainer, phase string) (string, error) {
	commands := ProcessLifecycleCommands(dc)
	
	var script strings.Builder
	script.WriteString("#!/bin/sh\n")
	script.WriteString("set -e\n\n")

	// Define the order of execution for each phase
	phaseCommands := map[string][]string{
		"create": {
			"initializeCommand",  // Runs on host
			"onCreateCommand",
			"updateContentCommand",
			"postCreateCommand",
		},
		"start": {
			"postStartCommand",
		},
		"attach": {
			"postAttachCommand",
		},
	}

	cmdList, ok := phaseCommands[phase]
	if !ok {
		return "", fmt.Errorf("unknown lifecycle phase: %s", phase)
	}

	for _, cmdName := range cmdList {
		if cmd, exists := commands[cmdName]; exists {
			script.WriteString(fmt.Sprintf("# %s\n", cmdName))
			
			if cmdName == "initializeCommand" {
				script.WriteString("# Note: This command should run on the host, not in the container\n")
			}
			
			shellCmd := cmd.ToShellCommand()
			if shellCmd != "" {
				script.WriteString(shellCmd)
				script.WriteString("\n\n")
			}
		}
	}

	return script.String(), nil
}

// HostRequirementsCheck checks if host meets the requirements
func HostRequirementsCheck(req *DevContainerCommonHostRequirements) error {
	if req == nil {
		return nil
	}

	// In a real implementation, we would check:
	// - CPU count
	// - Memory availability
	// - Storage space
	// - GPU availability
	
	// For now, just validate the format
	if req.Cpus != nil && *req.Cpus < 1 {
		return fmt.Errorf("invalid CPU requirement: %d", *req.Cpus)
	}

	if req.Memory != nil {
		// Memory format is validated by the schema
		// Could parse and check actual available memory here
	}

	if req.Storage != nil {
		// Storage format is validated by the schema
		// Could check actual available storage here
	}

	return nil
}