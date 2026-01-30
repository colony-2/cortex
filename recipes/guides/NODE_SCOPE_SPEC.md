# Node Scope Specification

## Purpose

Spell-out the scoping rules for recipe nodes (`op`, `sequence`, `state`) so recipe authors can reference inputs/outputs predictably when composing workflows.

## Node Types

- `op`: single activity or inline op.
- `sequence`: ordered list of child nodes.
- `state`: state machine composed of named states and transitions.

Each node owns its scope; children inherit from the container but cannot see into sibling containers unless outputs are explicitly exported.

## Scope Rules

### Sequence

*Children can access:*
- `inputs` passed to the sequence.
- Sibling outputs via `sequence.<node-id>.outputs.*`.
- Outputs defined on the sequence itself (`outputs:` block) once exported upward.

*Example*

```yaml
- id: normalize
  sequence:
    - id: fetch
      op: http_get
      inputs:
        url: '{{ inputs.api_root }}/users'
    - id: format
      op: command_execution
      inputs:
        run: "echo '{{ sequence.fetch.outputs.body }}' | jq '.[] | .name'"
  outputs:
    users_body: '{{ sequence.fetch.outputs.body }}'
```

### State Machine

*Children can access:*
- `inputs` passed to the state machine.
- Previously completed states via `states.<state-id>.outputs.*`.

*Example*

```yaml
state:
  initial: validate
  states:
    validate:
      op: validator
      transitions:
        - to: process
          when: 'states.validate.outputs.valid == true'
    process:
      op: handler
      inputs:
        payload: '{{ states.validate.outputs.cleaned }}'
outputs:
  final_status: '{{ states.process.outputs.status }}'
```

### Op

`op` nodes receive whatever is mapped into `inputs` and can set outputs by returning values from the activity/inline op.

## Mixing Node Types

You can compose nodes by nesting sequences/state machines. Always export data you need outside the container:

```yaml
- id: pipeline
  sequence:
    - id: prepare
      state:
        initial: bootstrap
        states:
          bootstrap:
            op: command_execution
            inputs:
              run: 'mkdir -p work'
            outputs:
              work_dir: 'work'
          complete:
            op: command_execution
            inputs:
              run: 'ls work'
            transitions:
              - to: done
        outputs:
          workspace: '{{ states.complete.outputs.work_dir }}'
    - id: summary
      op: command_execution
      inputs:
        run: "echo '{{ sequence.prepare.outputs.workspace }}'"
  outputs:
    workspace: '{{ sequence.prepare.outputs.workspace }}'
```

## Encapsulation Rules

1. Each container defines its own scope (`sequence`, `state`).
2. Children can reference siblings only via the container’s namespace (`sequence.<id>`, `states.<id>`).
3. To share data with outer nodes, declare an `outputs:` map on the current container.
4. Nesting requires importing context via `inputs` and exporting via `outputs` at each layer.

## Quick Reference

| Node type | Access to inputs | Access to siblings | Export mechanism |
|-----------|------------------|--------------------|------------------|
| `op`      | `inputs.*`       | N/A                | activity outputs |
| `sequence`| `inputs.*`       | `sequence.<id>.outputs.*` | `outputs:` block |
| `state`   | `inputs.*`       | `states.<id>.outputs.*`   | `outputs:` block |

Use this spec whenever you structure recipes that combine multiple node types, ensuring data flows remain explicit and predictable.
