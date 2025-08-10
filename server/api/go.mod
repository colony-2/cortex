module github.com/divisive-ai/vibethis/server/api

go 1.24.1

require (
	github.com/divisive-ai/vibethis/server/container v0.0.0
	github.com/divisive-ai/vibethis/server/core v0.0.0
	github.com/divisive-ai/vibethis/server/files v0.0.0
	github.com/divisive-ai/vibethis/server/git v0.0.0
	github.com/divisive-ai/vibethis/server/graph v0.0.0-00010101000000-000000000000
	github.com/divisive-ai/vibethis/server/openapi v0.0.0
	github.com/divisive-ai/vibethis/server/ops v0.0.0
	github.com/divisive-ai/vibethis/server/storage v0.0.0-00010101000000-000000000000
	github.com/gorilla/mux v1.8.0
	github.com/spf13/cobra v1.9.1
)

require (
	github.com/apapsch/go-jsonmerge/v2 v2.0.0 // indirect
	github.com/boltdb/bolt v1.3.1 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/facebookgo/clock v0.0.0-20150410010913-600d898af40a // indirect
	github.com/getkin/kin-openapi v0.132.0 // indirect
	github.com/go-chi/chi/v5 v5.2.2 // indirect
	github.com/go-openapi/jsonpointer v0.21.0 // indirect
	github.com/go-openapi/swag v0.23.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/mock v1.6.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/grpc-ecosystem/go-grpc-middleware v1.4.0 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.22.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/mohae/deepcopy v0.0.0-20170929034955-c48cc78d4826 // indirect
	github.com/nexus-rpc/sdk-go v0.3.0 // indirect
	github.com/oapi-codegen/runtime v1.1.2 // indirect
	github.com/oasdiff/yaml v0.0.0-20250309154309-f31be36b4037 // indirect
	github.com/oasdiff/yaml3 v0.0.0-20250309153720-d2182401db90 // indirect
	github.com/perimeterx/marshmallow v1.1.5 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/robfig/cron v1.2.0 // indirect
	github.com/spf13/pflag v1.0.6 // indirect
	github.com/stretchr/objx v0.5.2 // indirect
	github.com/stretchr/testify v1.10.0 // indirect
	go.temporal.io/api v1.49.1 // indirect
	go.temporal.io/sdk v1.35.0 // indirect
	golang.org/x/net v0.39.0 // indirect
	golang.org/x/sync v0.13.0 // indirect
	golang.org/x/sys v0.33.0 // indirect
	golang.org/x/text v0.24.0 // indirect
	golang.org/x/time v0.12.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20240903143218-8af14fe29dc1 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240903143218-8af14fe29dc1 // indirect
	google.golang.org/grpc v1.66.2 // indirect
	google.golang.org/protobuf v1.36.5 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace (
	github.com/divisive-ai/vibethis/server/container => ../container
	github.com/divisive-ai/vibethis/server/core => ../core
	github.com/divisive-ai/vibethis/server/files => ../files
	github.com/divisive-ai/vibethis/server/git => ../git
	github.com/divisive-ai/vibethis/server/graph => ../graph
	github.com/divisive-ai/vibethis/server/openapi => ../openapi
	github.com/divisive-ai/vibethis/server/ops => ../ops
	github.com/divisive-ai/vibethis/server/storage => ../storage
)
