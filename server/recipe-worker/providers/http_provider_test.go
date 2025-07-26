package providers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/divisive-ai/vibethis/server/recipe-worker/providers"
)

func TestHTTPProvider(t *testing.T) {
	t.Run("Execute GET request", func(t *testing.T) {
		// Create test server
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "GET", r.Method)
			assert.Equal(t, "value1", r.URL.Query().Get("param1"))
			
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"message": "success",
				"data":    "test data",
			})
		}))
		defer server.Close()
		
		// Create provider
		provider := providers.NewHTTPProvider()
		assert.Equal(t, "http", provider.GetType())
		
		// Execute request
		result, err := provider.Execute(context.Background(),
			map[string]interface{}{
				"url":    server.URL,
				"method": "GET",
			},
			map[string]interface{}{
				"queryParams": map[string]string{
					"param1": "value1",
				},
			})
		
		require.NoError(t, err)
		
		// Check result
		output, ok := result.(*providers.HTTPOutput)
		require.True(t, ok)
		assert.Equal(t, http.StatusOK, output.StatusCode)
		
		// Check response body
		body, ok := output.Body.(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "success", body["message"])
		assert.Equal(t, "test data", body["data"])
	})
	
	t.Run("Execute POST request with body", func(t *testing.T) {
		// Create test server
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "POST", r.Method)
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
			
			// Read request body
			var body map[string]interface{}
			err := json.NewDecoder(r.Body).Decode(&body)
			require.NoError(t, err)
			assert.Equal(t, "test value", body["field"])
			
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":      "123",
				"created": true,
			})
		}))
		defer server.Close()
		
		// Create provider
		provider := providers.NewHTTPProvider()
		
		// Execute request
		result, err := provider.Execute(context.Background(),
			map[string]interface{}{
				"url":    server.URL,
				"method": "POST",
				"headers": map[string]string{
					"Content-Type": "application/json",
				},
			},
			map[string]interface{}{
				"body": map[string]interface{}{
					"field": "test value",
				},
			})
		
		require.NoError(t, err)
		
		// Check result
		output, ok := result.(*providers.HTTPOutput)
		require.True(t, ok)
		assert.Equal(t, http.StatusCreated, output.StatusCode)
		
		// Check response body
		body, ok := output.Body.(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "123", body["id"])
		assert.Equal(t, true, body["created"])
	})
	
	t.Run("Handle timeout", func(t *testing.T) {
		// Create test server with delay
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// This will cause timeout
			<-r.Context().Done()
		}))
		defer server.Close()
		
		// Create provider
		provider := providers.NewHTTPProvider()
		
		// Execute request with short timeout
		_, err := provider.Execute(context.Background(),
			map[string]interface{}{
				"url":     server.URL,
				"method":  "GET",
				"timeout": 1, // 1 second timeout
			},
			map[string]interface{}{})
		
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "context deadline exceeded")
	})
	
	t.Run("Invalid config", func(t *testing.T) {
		provider := providers.NewHTTPProvider()
		
		// Execute with invalid config type
		_, err := provider.Execute(context.Background(),
			"invalid config type",
			map[string]interface{}{})
		
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid config type")
	})
	
	t.Run("Invalid input", func(t *testing.T) {
		provider := providers.NewHTTPProvider()
		
		// Execute with invalid input type
		_, err := provider.Execute(context.Background(),
			map[string]interface{}{
				"url":    "http://example.com",
				"method": "GET",
			},
			"invalid input type")
		
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid input type")
	})
	
	t.Run("Wrong number of arguments", func(t *testing.T) {
		provider := providers.NewHTTPProvider()
		
		// Execute with wrong number of arguments
		_, err := provider.Execute(context.Background(), map[string]interface{}{})
		
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "expected 2 arguments")
	})
}