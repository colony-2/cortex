package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"go.temporal.io/sdk/client"
	"go.uber.org/zap"
	recipe "github.com/vibethis/server/recipe-core/pkg/recipe"
	recipeworker "github.com/vibethis/server/recipe-worker"
	"github.com/vibethis/server/recipe-worker/pkg/worker"
	"github.com/vibethis/server/recipe-worker/providers"
)

// CustomDatabaseProvider implements a custom database activity provider
type CustomDatabaseProvider struct{}

func (p *CustomDatabaseProvider) GetType() string {
	return "database"
}

func (p *CustomDatabaseProvider) Execute(ctx context.Context, args ...interface{}) (interface{}, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("expected 2 arguments (config, input), got %d", len(args))
	}
	
	config, ok := args[0].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid config type")
	}
	
	input, ok := args[1].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid input type")
	}
	
	// Simulate database operation
	operation := config["operation"].(string)
	table := config["table"].(string)
	
	switch operation {
	case "insert":
		return map[string]interface{}{
			"success": true,
			"id":      "generated-id-123",
			"message": fmt.Sprintf("Inserted into %s", table),
		}, nil
	case "query":
		return map[string]interface{}{
			"success": true,
			"rows": []map[string]interface{}{
				{"id": "1", "name": "Item 1"},
				{"id": "2", "name": "Item 2"},
			},
			"count": 2,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported operation: %s", operation)
	}
}

// WeatherProvider implements a weather API activity provider using generics
type WeatherConfig struct {
	APIKey   string `json:"apiKey"`
	Endpoint string `json:"endpoint"`
}

type WeatherInput struct {
	City    string `json:"city"`
	Country string `json:"country"`
}

type WeatherOutput struct {
	Temperature float64 `json:"temperature"`
	Conditions  string  `json:"conditions"`
	Humidity    int     `json:"humidity"`
}

func getWeather(ctx context.Context, config WeatherConfig, input WeatherInput) (WeatherOutput, error) {
	// Simulate API call
	log.Printf("Fetching weather for %s, %s", input.City, input.Country)
	
	// In a real implementation, you would call the weather API here
	return WeatherOutput{
		Temperature: 22.5,
		Conditions:  "Partly cloudy",
		Humidity:    65,
	}, nil
}

func main() {
	// Create logger
	logger, _ := zap.NewDevelopment()
	
	// Create Temporal client (you need Temporal server running)
	temporalClient, err := client.Dial(client.Options{
		HostPort: client.DefaultHostPort,
	})
	if err != nil {
		log.Fatal("Unable to create Temporal client", err)
	}
	defer temporalClient.Close()
	
	// Create worker manager
	workerManager := worker.NewWorkerManager(logger, temporalClient)
	
	// Register custom providers
	
	// 1. Register database provider
	dbProvider := &CustomDatabaseProvider{}
	if err := workerManager.RegisterProvider(dbProvider); err != nil {
		log.Fatal("Failed to register database provider:", err)
	}
	
	// 2. Register HTTP provider (built-in)
	httpProvider := providers.NewHTTPProvider()
	if err := workerManager.RegisterProvider(httpProvider); err != nil {
		log.Fatal("Failed to register HTTP provider:", err)
	}
	
	// 3. Register weather provider using typed provider
	weatherProvider := recipeworker.NewTypedProvider("weather", getWeather)
	if err := workerManager.RegisterProvider(weatherProvider); err != nil {
		log.Fatal("Failed to register weather provider:", err)
	}
	
	// Create a sample recipe that uses these activities
	sampleRecipe := &recipe.Recipe{
		Name:    "weather-data-processor",
		Version: "1.0.0",
		Activities: []recipe.ActivityDefinition{
			{
				Name:        "fetch-weather",
				Description: "Fetch current weather data",
				Implementation: recipe.Implementation{
					Type: "weather",
					Config: map[string]interface{}{
						"apiKey":   "demo-key",
						"endpoint": "https://api.weather.com",
					},
				},
				Inputs: map[string]interface{}{
					"city":    "${ inputs.city }",
					"country": "${ inputs.country }",
				},
				Outputs: map[string]interface{}{
					"temperature": "${ outputs.temperature }",
					"conditions":  "${ outputs.conditions }",
				},
			},
			{
				Name:        "store-weather",
				Description: "Store weather data in database",
				Implementation: recipe.Implementation{
					Type: "database",
					Config: map[string]interface{}{
						"operation": "insert",
						"table":     "weather_data",
					},
				},
				Inputs: map[string]interface{}{
					"temperature": "${ steps.fetch-weather.outputs.temperature }",
					"conditions":  "${ steps.fetch-weather.outputs.conditions }",
					"city":        "${ inputs.city }",
				},
			},
			{
				Name:        "notify-api",
				Description: "Notify external API about new weather data",
				Implementation: recipe.Implementation{
					Type: "http",
					Config: map[string]interface{}{
						"url":    "https://webhook.site/test",
						"method": "POST",
						"headers": map[string]string{
							"Content-Type": "application/json",
						},
					},
				},
				Inputs: map[string]interface{}{
					"body": map[string]interface{}{
						"event": "weather_updated",
						"data": map[string]interface{}{
							"city":        "${ inputs.city }",
							"temperature": "${ steps.fetch-weather.outputs.temperature }",
						},
					},
				},
			},
		},
		Workflow: &recipe.WorkflowDefinition{
			Name: "process-weather-data",
			Type: "sequential",
			Steps: []recipe.WorkflowStep{
				{ID: "fetch-weather", Activity: "fetch-weather"},
				{ID: "store-weather", Activity: "store-weather"},
				{ID: "notify-api", Activity: "notify-api"},
			},
		},
	}
	
	// Start worker for the recipe
	if err := workerManager.StartWorker(sampleRecipe); err != nil {
		log.Fatal("Failed to start worker:", err)
	}
	
	log.Println("Worker started successfully with custom providers!")
	log.Println("Available activity types:")
	log.Println("- database (custom)")
	log.Println("- weather (custom typed)")
	log.Println("- http (built-in)")
	
	// Keep worker running
	time.Sleep(5 * time.Minute)
	
	// Stop all workers
	workerManager.StopAll()
}