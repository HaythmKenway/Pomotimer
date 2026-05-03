# Configuration Guide

PomoTimer uses a `config.json` file located in the root directory. If it doesn't exist, it will be created with default values on the first run.

## Example Configuration
```json
{
  "steps": [
    {
      "name": "Focus",
      "duration_minutes": 25,
      "requires_recording": true
    },
    {
      "name": "Short Break",
      "duration_minutes": 5,
      "requires_recording": false
    }
  ]
}
```

## Fields
- **name**: The display name of the timer step.
- **duration_minutes**: Length of the timer in minutes.
- **requires_recording**: If `true`, a popup will appear at the end of the session asking "What did you do?".
