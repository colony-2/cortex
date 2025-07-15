# server/agent

## Directory Purpose

The `server/agent` directory is a placeholder module for future agent functionality in the vibethis system. Currently, it contains only a go.mod file defining the module as `vibethis/agent` but no implementation.

## Current State

- **Status**: Not implemented
- **Module**: `vibethis/agent`
- **Go Version**: 1.24.1

## Intended Architecture

Based on the agent concepts used in the ono server's recipe system, this module would likely implement:

### Agent Definition
Agents in vibethis represent autonomous entities with:
- **Role**: Specific function or expertise (e.g., "Senior Research Analyst", "Data Analyst")
- **Capabilities**: List of actions the agent can perform
- **Goals**: Objectives the agent works towards
- **Constraints**: Rules and limitations for agent behavior

### Integration Points

1. **With ono server**: The ono recipe system already supports agent definitions in workflow orchestration
2. **With core interfaces**: Would implement core.Agent or similar interfaces from server/core
3. **With activity executors**: Agents would execute activities through the established activity system

## Relationship to Existing Systems

### Ono Recipe System
The ono server (server/ono) already uses agent concepts in its workflow recipes:
- Agents are defined in YAML files within recipe directories
- Each agent has role, capabilities, goals, and constraints
- Agents participate in workflow execution

### Example Agent Structure (from ono)
```yaml
agents:
  researcher:
    role: Senior Research Analyst
    capabilities:
      - web_search
      - data_extraction
      - source_validation
    goals:
      - Find comprehensive, accurate information
      - Identify credible sources
```

## Future Implementation Considerations

When implementing this module, consider:

1. **Agent Runtime**: How agents execute tasks and maintain state
2. **Communication**: Inter-agent messaging and coordination
3. **Persistence**: Agent state storage and recovery
4. **Security**: Agent permission models and sandboxing
5. **Monitoring**: Agent activity tracking and debugging

## Related Directories

- `/agent/rucc`: Contains Claude Code project reference documentation
- `/server/ono`: Contains existing agent definitions in recipe system
- `/server/core`: Would provide interfaces for agent implementation

## Notes for Implementation

- Review existing agent definitions in ono recipe examples
- Coordinate with ono workflow system for integration
- Consider event-driven architecture for agent communication
- Plan for scalability with multiple concurrent agents