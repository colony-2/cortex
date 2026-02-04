# Bug: llm_inference2 response_schema outputs fail validation when using json_parse

## Summary
When an llm_inference2 step returns a response with `response_schema` defined, the op runtime returns a stringified JSON object (as designed). However, during recipe **validation**, accessing `sequence.<step>.outputs.response` with `json_parse(...)` is rejected: the validator reports that the value is not a string. This prevents authors from using the documented access pattern `json_parse(sequence.assess_cell.outputs.response).field` in recipe outputs.

## Minimal Reproduction
Recipe (`tmp-test.yaml`):
```yaml
id: tmp-test
sequence:
  - id: call
    op: llm_inference2
    inputs:
      default_provider: openai
      default_model: gpt-4.1
      temperature: 0
      tools: []
      response_schema:
        type: object
        properties:
          ok:
            type: boolean
          note:
            type: string
        required: [ok, note]
        additionalProperties: false
      system_prompt: "Return ONLY JSON"
      prompt: "say ok true"
outputs:
  ok: "{{ json_parse(sequence.call.outputs.response).ok }}"
```

Validation command:
```
c2 recipe validate --name tmp-test --content-file tmp-test.yaml
```

Observed error:
```
failed to resolve sequence outputs: failed to resolve input 'ok': failed to evaluate CEL expression: json_parse: expected string
```

## Notes
- Runtime behavior shows the op returns a stringified JSON object (example from `repro-llm-schema` run): `{"note":"Request received and processed successfully.","ok":true}`. So the issue appears limited to validation/type inference for `sequence.<step>.outputs.response`.
- Expected: validation should treat `sequence.<step>.outputs.response` as a string when `response_schema` is present, allowing `json_parse(...)` access without guards.
