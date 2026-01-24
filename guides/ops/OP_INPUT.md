# input Op

Collects user input either as a single question or as a multi-field form; returns the response and user metadata.

## Input Structure

```json
{
  "form": {
    "question": "What should the title be?",
    "type": "short_answer",
    "options": [],
    "scale": null,
    "title": "Optional form title",
    "fields": [
      {
        "id": "priority",
        "type": "dropdown",
        "question": "Priority",
        "options": [
          { "value": "low", "label": "Low" },
          { "value": "high", "label": "High" }
        ]
      }
    ],
    "context": {
      "artifacts": [],
      "artifacts_from_output": ""
    },
    "timeout": 300,
    "default_on_timeout": null
  }
}
```

## Output Structure

```json
{
  "response": "single-answer",
  "fields": {
    "priority": "high"
  },
  "user_id": "user-123",
  "metadata": {
    "source": "ui"
  }
}
```
