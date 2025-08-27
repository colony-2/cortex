package: oas
generate:
  models: true
output-options:
  user-templates:
    union.tmpl: |-
      {{range .Types}}
          {{$typeName := .TypeName -}}
          {{$discriminator := .Schema.Discriminator}}
          {{$properties := .Schema.Properties -}}

          // Internal cached fields for each union type
          {{range .Schema.UnionElements}}
              {{$element := . -}}
              // cached{{.Method}} holds the unmarshaled {{.}} if valid
              var cached{{.Method}} *{{.}}
              var cached{{.Method}}Valid *bool
          {{end}}

          {{range .Schema.UnionElements}}
              {{$element := . -}}
              // As{{ .Method }} returns the union data inside the {{$typeName}} as a {{.}}
              func (t {{$typeName}}) As{{ .Method }}() ({{.}}, error) {
                  // Use cached version if available
                  if t.cached{{.Method}}Valid != nil && *t.cached{{.Method}}Valid {
                      return *t.cached{{.Method}}, nil
                  }

                  var body {{.}}
                  err := json.Unmarshal(t.union, &body)
                  if err != nil {
                      return body, err
                  }

                  // Cache the result
                  t.cached{{.Method}} = &body
                  valid := true
                  t.cached{{.Method}}Valid = &valid

                  return body, nil
              }

              // Is{{ .Method }} checks if the union data represents a {{.}}
              func (t {{$typeName}}) Is{{ .Method }}() bool {
                  // Use cached validation if available
                  if t.cached{{.Method}}Valid != nil {
                      return *t.cached{{.Method}}Valid
                  }

                  var body {{.}}
                  err := json.Unmarshal(t.union, &body)
                  isValid := err == nil

                  {{if $discriminator -}}
                      {{range $value, $type := $discriminator.Mapping -}}
                          {{if eq $type $element -}}
                              // Additional discriminator check
                              if isValid {
                                  discriminator, discErr := t.Discriminator()
                                  isValid = discErr == nil && discriminator == "{{$value}}"
                              }
                          {{end -}}
                      {{end -}}
                  {{end}}

                  // Cache the result
                  if isValid {
                      t.cached{{.Method}} = &body
                  }
                  t.cached{{.Method}}Valid = &isValid

                  return isValid
              }

              // From{{ .Method }} overwrites any union data inside the {{$typeName}} as the provided {{.}}
              func (t *{{$typeName}}) From{{ .Method }} (v {{.}}) error {
                  // Clear cache
                  t.cached{{.Method}} = nil
                  t.cached{{.Method}}Valid = nil

                  {{if $discriminator -}}
                      {{range $value, $type := $discriminator.Mapping -}}
                          {{if eq $type $element -}}
                              {{$hasProperty := false -}}
                              {{range $properties -}}
                                  {{if eq .GoFieldName $discriminator.PropertyName -}}
                                      t.{{$discriminator.PropertyName}} = "{{$value}}"
                                      {{$hasProperty = true -}}
                                  {{end -}}
                              {{end -}}
                              {{if not $hasProperty}}v.{{$discriminator.PropertyName}} = "{{$value}}"{{end}}
                          {{end -}}
                      {{end -}}
                  {{end -}}
                  b, err := json.Marshal(v)
                  t.union = b

                  // Set cache
                  if err == nil {
                      t.cached{{.Method}} = &v
                      valid := true
                      t.cached{{.Method}}Valid = &valid
                  }

                  return err
              }

              // Merge{{ .Method }} performs a merge with any union data inside the {{$typeName}}, using the provided {{.}}
              func (t *{{$typeName}}) Merge{{ .Method }} (v {{.}}) error {
                  // Clear cache since we're modifying
                  t.cached{{.Method}} = nil
                  t.cached{{.Method}}Valid = nil

                  {{if $discriminator -}}
                      {{range $value, $type := $discriminator.Mapping -}}
                          {{if eq $type $element -}}
                              {{$hasProperty := false -}}
                              {{range $properties -}}
                                  {{if eq .GoFieldName $discriminator.PropertyName -}}
                                      t.{{$discriminator.PropertyName}} = "{{$value}}"
                                      {{$hasProperty = true -}}
                                  {{end -}}
                              {{end -}}
                              {{if not $hasProperty}}v.{{$discriminator.PropertyName}} = "{{$value}}"{{end}}
                          {{end -}}
                      {{end -}}
                  {{end -}}
                  b, err := json.Marshal(v)
                  if err != nil {
                    return err
                  }

                  merged, err := runtime.JSONMerge(t.union, b)
                  t.union = merged
                  return err
              }
          {{end}}

          // UnmarshalJSON supports JSON unmarshaling
          func (t *{{.TypeName}}) UnmarshalJSON(b []byte) error {
              // Clear all caches
              {{range .Schema.UnionElements}}
              t.cached{{.Method}} = nil
              t.cached{{.Method}}Valid = nil
              {{end}}

              err := t.union.UnmarshalJSON(b)
              {{if ne 0 (len .Schema.Properties) -}}
                  if err != nil {
                      return err
                  }
                  object := make(map[string]json.RawMessage)
                  err = json.Unmarshal(b, &object)
                  if err != nil {
                      return err
                  }
                  {{range .Schema.Properties}}
                      if raw, found := object["{{.JsonFieldName}}"]; found {
                          err = json.Unmarshal(raw, &t.{{.GoFieldName}})
                          if err != nil {
                              return fmt.Errorf("error reading '{{.JsonFieldName}}': %w", err)
                          }
                      }
                  {{end}}
              {{end -}}

              // Pre-validate all types during unmarshal
              {{range .Schema.UnionElements}}
              t.Is{{.Method}}() // This will populate the cache
              {{end}}

              return err
          }

          // UnmarshalYAML supports YAML unmarshaling
          func (t *{{.TypeName}}) UnmarshalYAML(value *yaml.Node) error {
              // Clear all caches
              {{range .Schema.UnionElements}}
              t.cached{{.Method}} = nil
              t.cached{{.Method}}Valid = nil
              {{end}}

              // Convert YAML to JSON first, then use JSON unmarshaling
              jsonBytes, err := yaml.Marshal(value)
              if err != nil {
                  return err
              }

              return t.UnmarshalJSON(jsonBytes)
          }

          {{if $discriminator}}
              func (t {{.TypeName}}) Discriminator() (string, error) {
                  var discriminator struct {
                      Discriminator string {{$discriminator.JSONTag}}
                  }
                  err := json.Unmarshal(t.union, &discriminator)
                  return discriminator.Discriminator, err
              }

              {{if ne 0 (len $discriminator.Mapping)}}
                  func (t {{.TypeName}}) ValueByDiscriminator() (interface{}, error) {
                      discriminator, err := t.Discriminator()
                      if err != nil {
                          return nil, err
                      }
                      switch discriminator{
                          {{range $value, $type := $discriminator.Mapping -}}
                              case "{{$value}}":
                                  return t.As{{$type}}()
                          {{end -}}
                          default:
                              return nil, errors.New("unknown discriminator value: "+discriminator)
                      }
                  }
              {{end}}
          {{end}}

          {{if not .Schema.HasAdditionalProperties}}
          func (t {{.TypeName}}) MarshalJSON() ([]byte, error) {
              b, err := t.union.MarshalJSON()
              {{if ne 0 (len .Schema.Properties) -}}
                  if err != nil {
                      return nil, err
                  }
                  object := make(map[string]json.RawMessage)
                  if t.union != nil {
                    err = json.Unmarshal(b, &object)
                    if err != nil {
                      return nil, err
                    }
                  }
                  {{range .Schema.Properties}}
                  {{if .HasOptionalPointer}}if t.{{.GoFieldName}} != nil { {{end}}
                      object["{{.JsonFieldName}}"], err = json.Marshal(t.{{.GoFieldName}})
                      if err != nil {
                          return nil, fmt.Errorf("error marshaling '{{.JsonFieldName}}': %w", err)
                      }
                  {{if .HasOptionalPointer}} }{{end}}
                  {{end -}}
                  b, err = json.Marshal(object)
              {{end -}}
              return b, err
          }
          {{end}}
      {{end}}
  disable-type-aliases-for-type: []
  overlay:
    path: ""
