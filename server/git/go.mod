module github.com/divisive-ai/vibethis/server/git

go 1.24.1

replace github.com/divisive-ai/vibethis/server/core => ../core

replace github.com/divisive-ai/vibethis/server/recipe-core => ../recipe-core

replace github.com/divisive-ai/vibethis/server/llm => ../llm

replace github.com/divisive-ai/vibethis/server/ops => ../ops

require (
	github.com/divisive-ai/vibethis/server/core v0.0.0-00010101000000-000000000000
	github.com/divisive-ai/vibethis/server/recipe-core v0.0.0-00010101000000-000000000000
	github.com/stretchr/testify v1.11.1
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/fatih/structs v1.1.0 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.1.5 // indirect
	github.com/kr/pretty v0.3.1 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/rogpeppe/go-internal v1.11.0 // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	gorm.io/gorm v1.25.10 // indirect
)
