module github.com/divisive-ai/vibethis/server/api

go 1.24

toolchain go1.24.1

require (
	github.com/divisive-ai/vibethis/server/container v0.0.0
	github.com/divisive-ai/vibethis/server/core v0.0.0
	github.com/divisive-ai/vibethis/server/files v0.0.0
	github.com/divisive-ai/vibethis/server/git v0.0.0
	github.com/divisive-ai/vibethis/server/graph v0.0.0-00010101000000-000000000000
	github.com/divisive-ai/vibethis/server/openapi v0.0.0
	github.com/divisive-ai/vibethis/server/storage v0.0.0-00010101000000-000000000000
	github.com/gorilla/mux v1.8.0
	github.com/spf13/cobra v1.9.1
)

require (
	github.com/apapsch/go-jsonmerge/v2 v2.0.0 // indirect
	github.com/boltdb/bolt v1.3.1 // indirect
	github.com/getkin/kin-openapi v0.132.0 // indirect
	github.com/go-openapi/jsonpointer v0.21.0 // indirect
	github.com/go-openapi/swag v0.23.0 // indirect
	github.com/google/uuid v1.5.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/mohae/deepcopy v0.0.0-20170929034955-c48cc78d4826 // indirect
	github.com/oapi-codegen/runtime v1.1.2 // indirect
	github.com/oasdiff/yaml v0.0.0-20250309154309-f31be36b4037 // indirect
	github.com/oasdiff/yaml3 v0.0.0-20250309153720-d2182401db90 // indirect
	github.com/perimeterx/marshmallow v1.1.5 // indirect
	github.com/spf13/pflag v1.0.6 // indirect
	golang.org/x/sys v0.33.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace (
	github.com/divisive-ai/vibethis/server/container => ../container
	github.com/divisive-ai/vibethis/server/core => ../core
	github.com/divisive-ai/vibethis/server/files => ../files
	github.com/divisive-ai/vibethis/server/git => ../git
	github.com/divisive-ai/vibethis/server/graph => ../graph
	github.com/divisive-ai/vibethis/server/openapi => ../openapi
	github.com/divisive-ai/vibethis/server/storage => ../storage
)
