package executor

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/worker"
	"go.uber.org/zap"
)

// ActivityExecutor handles the execution of activities using reflection
type ActivityExecutor struct {
	registry *worker.ActivityRegistry
	logger   *zap.Logger
}

// NewActivityExecutor creates a new activity executor
func NewActivityExecutor(registry *worker.ActivityRegistry, logger *zap.Logger) *ActivityExecutor {
	return &ActivityExecutor{
		registry: registry,
		logger:   logger,
	}
}

// CreateTemporalActivity creates a Temporal-compatible activity function for the given activity type
func (e *ActivityExecutor) CreateTemporalActivity(activityType string) interface{} {
	return func(ctx context.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
		e.logger.Debug("Executing activity",
			zap.String("type", activityType),
			zap.Any("inputs", inputs),
		)
		
		// Get activity from registry
		activityReg, exists := e.registry.Get(activityType)
		if !exists {
			return nil, fmt.Errorf("activity type %s not found", activityType)
		}
		
		// Use reflection to invoke the activity's Execute method
		if activityReg.Activity != nil {
			output, err := e.invokeActivity(ctx, activityReg.Activity, inputs)
			if err != nil {
				return nil, err
			}
			
			e.logger.Debug("Activity completed",
				zap.String("type", activityType),
				zap.Any("outputs", output),
			)
			
			return output, nil
		}
		
		// Fallback: return a simple success response
		outputs := map[string]interface{}{
			"result": "success",
		}
		
		e.logger.Debug("Activity completed with fallback",
			zap.String("type", activityType),
			zap.Any("outputs", outputs),
		)
		
		return outputs, nil
	}
}

// invokeActivity uses reflection to call the Execute method on an activity
func (e *ActivityExecutor) invokeActivity(
	ctx context.Context,
	activity interface{},
	inputs map[string]interface{},
) (map[string]interface{}, error) {
	activityValue := reflect.ValueOf(activity)
	executeMethod := activityValue.MethodByName("Execute")
	
	if !executeMethod.IsValid() {
		return nil, fmt.Errorf("activity does not have Execute method")
	}
	
	// The Execute method signature is: Execute(ctx context.Context, config ConfigType, input InputType) (OutputType, error)
	// We need to create the appropriate config and input types
	
	// Get the method type to understand the parameter types
	methodType := executeMethod.Type()
	if methodType.NumIn() != 3 || methodType.NumOut() != 2 {
		return nil, fmt.Errorf("Execute method has unexpected signature")
	}
	
	// Create zero values for config and input parameters
	configType := methodType.In(1)
	inputType := methodType.In(2)
	
	configValue := reflect.New(configType).Elem()
	inputValue := reflect.New(inputType).Elem()
	
	// Try to populate config struct from inputs map (if config fields are provided)
	if err := e.populateStruct(configValue, configType, inputs, "config."); err != nil {
		e.logger.Warn("Failed to populate config struct", zap.Error(err))
	}
	
	// Try to populate the input struct from the inputs map
	if err := e.populateStruct(inputValue, inputType, inputs, ""); err != nil {
		e.logger.Warn("Failed to populate input struct", zap.Error(err))
	}
	
	// Call the Execute method
	results := executeMethod.Call([]reflect.Value{
		reflect.ValueOf(ctx),
		configValue,
		inputValue,
	})
	
	// Check for error
	if len(results) == 2 {
		if !results[1].IsNil() {
			// There was an error
			if err, ok := results[1].Interface().(error); ok {
				return nil, err
			}
		}
		
		// Convert the output to a map
		return e.structToMap(results[0])
	}
	
	return nil, fmt.Errorf("Execute method returned unexpected number of results")
}

// populateStruct populates a struct from a map of values
func (e *ActivityExecutor) populateStruct(
	structValue reflect.Value,
	structType reflect.Type,
	values map[string]interface{},
	prefix string,
) error {
	if structType.Kind() != reflect.Struct {
		return nil // Not a struct, nothing to populate
	}
	
	for i := 0; i < structType.NumField(); i++ {
		field := structType.Field(i)
		jsonTag := field.Tag.Get("json")
		if jsonTag == "" || jsonTag == "-" {
			continue
		}
		
		tagName := strings.Split(jsonTag, ",")[0]
		if tagName == "" {
			continue
		}
		
		// Look for the value in the map
		// First try with prefix if provided
		var val interface{}
		var ok bool
		
		if prefix != "" {
			val, ok = values[prefix+tagName]
		}
		if !ok {
			val, ok = values[tagName]
		}
		
		if ok {
			fieldValue := structValue.Field(i)
			if fieldValue.CanSet() {
				if err := e.setFieldValue(fieldValue, val); err != nil {
					e.logger.Debug("Failed to set field value",
						zap.String("field", field.Name),
						zap.Error(err),
					)
				}
			}
		}
		
		// Handle nested structs
		if field.Type.Kind() == reflect.Struct {
			fieldValue := structValue.Field(i)
			if err := e.populateStruct(fieldValue, field.Type, values, prefix+tagName+"."); err != nil {
				return err
			}
		}
	}
	
	return nil
}

// setFieldValue sets a field value with type conversion
func (e *ActivityExecutor) setFieldValue(field reflect.Value, value interface{}) error {
	if value == nil {
		return nil
	}
	
	valueType := reflect.TypeOf(value)
	fieldType := field.Type()
	
	// Direct assignment if types match
	if valueType.AssignableTo(fieldType) {
		field.Set(reflect.ValueOf(value))
		return nil
	}
	
	// Handle common conversions
	switch fieldType.Kind() {
	case reflect.String:
		if str, ok := value.(string); ok {
			field.SetString(str)
			return nil
		}
		field.SetString(fmt.Sprintf("%v", value))
		return nil
		
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		switch v := value.(type) {
		case float64:
			field.SetInt(int64(v))
			return nil
		case int:
			field.SetInt(int64(v))
			return nil
		case int64:
			field.SetInt(v)
			return nil
		}
		
	case reflect.Float32, reflect.Float64:
		switch v := value.(type) {
		case float64:
			field.SetFloat(v)
			return nil
		case float32:
			field.SetFloat(float64(v))
			return nil
		case int:
			field.SetFloat(float64(v))
			return nil
		}
		
	case reflect.Bool:
		if b, ok := value.(bool); ok {
			field.SetBool(b)
			return nil
		}
		
	case reflect.Map:
		if m, ok := value.(map[string]interface{}); ok {
			mapValue := reflect.MakeMap(fieldType)
			for k, v := range m {
				keyValue := reflect.ValueOf(k)
				elemValue := reflect.ValueOf(v)
				if elemValue.Type().AssignableTo(fieldType.Elem()) {
					mapValue.SetMapIndex(keyValue, elemValue)
				}
			}
			field.Set(mapValue)
			return nil
		}
		
	case reflect.Slice:
		if slice, ok := value.([]interface{}); ok {
			sliceValue := reflect.MakeSlice(fieldType, len(slice), len(slice))
			for i, item := range slice {
				if err := e.setFieldValue(sliceValue.Index(i), item); err != nil {
					return err
				}
			}
			field.Set(sliceValue)
			return nil
		}
	}
	
	return fmt.Errorf("cannot convert %v to %v", valueType, fieldType)
}

// structToMap converts a struct to a map
func (e *ActivityExecutor) structToMap(value reflect.Value) (map[string]interface{}, error) {
	if value.Kind() != reflect.Struct {
		// If it's not a struct, wrap it in a map
		return map[string]interface{}{
			"result": value.Interface(),
		}, nil
	}
	
	outputMap := make(map[string]interface{})
	outputType := value.Type()
	
	for i := 0; i < value.NumField(); i++ {
		field := outputType.Field(i)
		jsonTag := field.Tag.Get("json")
		if jsonTag == "" || jsonTag == "-" {
			continue
		}
		
		tagName := strings.Split(jsonTag, ",")[0]
		if tagName == "" {
			continue
		}
		
		fieldValue := value.Field(i)
		if fieldValue.CanInterface() {
			// Handle nested structs
			if fieldValue.Kind() == reflect.Struct {
				nestedMap, err := e.structToMap(fieldValue)
				if err != nil {
					return nil, err
				}
				outputMap[tagName] = nestedMap
			} else {
				outputMap[tagName] = fieldValue.Interface()
			}
		}
	}
	
	return outputMap, nil
}