package recipe

// Example of how external modules can register custom activity types

/*
// Example 1: Strict Database Activity Type
func RegisterDatabaseActivity(registry *ActivityTypeRegistry) error {
	return registry.RegisterActivityType(&ActivityTypeDefinition{
		Type:        "database",
		Description: "Executes database queries with connection pooling",
		ConfigSchema: JSONSchema{
			"type":     "object",
			"required": []string{"datasource", "query"},
			"properties": map[string]interface{}{
				"datasource": map[string]interface{}{
					"type": "string",
					"enum": []string{"postgres", "mysql", "mongodb"},
				},
				"query": map[string]interface{}{
					"type": "string",
				},
				"connectionString": map[string]interface{}{
					"type":   "string",
					"format": "uri",
				},
				"timeout": map[string]interface{}{
					"type":    "integer",
					"minimum": 1,
					"maximum": 300,
				},
			},
			"additionalProperties": false,
		},
		InputSchema: JSONSchema{
			"type": "object",
			"properties": map[string]interface{}{
				"parameters": map[string]interface{}{
					"type": "object",
					"additionalProperties": map[string]interface{}{
						"oneOf": []interface{}{
							map[string]interface{}{"type": "string"},
							map[string]interface{}{"type": "number"},
							map[string]interface{}{"type": "boolean"},
							map[string]interface{}{"type": "null"},
						},
					},
				},
			},
			"additionalProperties": false,
		},
		OutputSchema: JSONSchema{
			"type": "object",
			"properties": map[string]interface{}{
				"rows": map[string]interface{}{
					"type": "array",
					"items": map[string]interface{}{
						"type": "object",
					},
				},
				"rowCount": map[string]interface{}{
					"type": "integer",
				},
			},
			"required":             []string{"rows", "rowCount"},
			"additionalProperties": false,
		},
		RequiredConfig:         true,
		AllowAdditionalConfig:  false,
		AllowAdditionalInputs:  false,
		AllowAdditionalOutputs: false,
	})
}

// Example 2: Flexible Webhook Activity Type  
func RegisterWebhookActivity(registry *ActivityTypeRegistry) error {
	return registry.RegisterActivityType(&ActivityTypeDefinition{
		Type:        "webhook",
		Description: "Sends data to webhooks with flexible payloads",
		ConfigSchema: JSONSchema{
			"type":     "object",
			"required": []string{"url"},
			"properties": map[string]interface{}{
				"url": map[string]interface{}{
					"type":   "string",
					"format": "uri",
				},
				"method": map[string]interface{}{
					"type":    "string",
					"default": "POST",
					"enum":    []string{"POST", "PUT", "PATCH"},
				},
				"headers": map[string]interface{}{
					"type": "object",
					"additionalProperties": map[string]interface{}{
						"type": "string",
					},
				},
			},
		},
		// No InputSchema - accepts any input
		InputSchema: nil,
		// No OutputSchema - returns any output
		OutputSchema:           nil,
		RequiredConfig:         true,
		AllowAdditionalConfig:  true,
		AllowAdditionalInputs:  true,
		AllowAdditionalOutputs: true,
	})
}

// Example 3: Email Activity with Moderate Strictness
func RegisterEmailActivity(registry *ActivityTypeRegistry) error {
	return registry.RegisterActivityType(&ActivityTypeDefinition{
		Type:        "email",
		Description: "Sends emails with template support",
		ConfigSchema: JSONSchema{
			"type":     "object",
			"required": []string{"smtp_server"},
			"properties": map[string]interface{}{
				"smtp_server": map[string]interface{}{
					"type": "string",
				},
				"smtp_port": map[string]interface{}{
					"type":    "integer",
					"default": 587,
				},
				"use_tls": map[string]interface{}{
					"type":    "boolean",
					"default": true,
				},
				"template": map[string]interface{}{
					"type": "string",
				},
			},
		},
		InputSchema: JSONSchema{
			"type":     "object",
			"required": []string{"to", "subject"},
			"properties": map[string]interface{}{
				"to": map[string]interface{}{
					"oneOf": []interface{}{
						map[string]interface{}{
							"type":   "string",
							"format": "email",
						},
						map[string]interface{}{
							"type": "array",
							"items": map[string]interface{}{
								"type":   "string",
								"format": "email",
							},
						},
					},
				},
				"cc": map[string]interface{}{
					"type": "array",
					"items": map[string]interface{}{
						"type":   "string",
						"format": "email",
					},
				},
				"subject": map[string]interface{}{
					"type": "string",
				},
				"body": map[string]interface{}{
					"type": "string",
				},
				"attachments": map[string]interface{}{
					"type": "array",
					"items": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"filename": map[string]interface{}{"type": "string"},
							"content":  map[string]interface{}{"type": "string"},
							"encoding": map[string]interface{}{
								"type":    "string",
								"enum":    []string{"base64", "utf-8"},
								"default": "utf-8",
							},
						},
						"required": []string{"filename", "content"},
					},
				},
			},
			"additionalProperties": true, // Allow extra fields for template variables
		},
		OutputSchema: JSONSchema{
			"type": "object",
			"properties": map[string]interface{}{
				"messageId": map[string]interface{}{
					"type": "string",
				},
				"timestamp": map[string]interface{}{
					"type":   "string",
					"format": "date-time",
				},
			},
			"required": []string{"messageId"},
		},
		RequiredConfig:         true,
		AllowAdditionalConfig:  true,
		AllowAdditionalInputs:  true,
		AllowAdditionalOutputs: true,
	})
}

// Example Usage in YAML:

// activities:
//   - name: query_users
//     description: Get active users from database
//     implementation:
//       type: database
//       config:
//         datasource: postgres
//         connectionString: ${DATABASE_URL}
//         query: "SELECT * FROM users WHERE active = $1"
//         timeout: 30
//     inputs:
//       - name: active_only
//         type: boolean
//         default: true
//     outputs:
//       - name: users
//         type: array
//
//   - name: notify_webhook
//     description: Send notification to external system
//     implementation:
//       type: webhook
//       config:
//         url: ${WEBHOOK_URL}
//         headers:
//           Authorization: "Bearer ${WEBHOOK_TOKEN}"
//
//   - name: send_welcome_email
//     description: Send welcome email to new user
//     implementation:
//       type: email
//       config:
//         smtp_server: smtp.gmail.com
//         template: "welcome_template.html"
//     inputs:
//       - name: user_email
//         type: string
//       - name: user_name
//         type: string
*/