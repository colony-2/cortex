module github.com/divisive-ai/vibethis/server/recipe-worker

go 1.24.1

require (
	github.com/davecgh/go-spew v1.1.1
	github.com/divisive-ai/vibethis/server/git v0.0.0-00010101000000-000000000000
	github.com/divisive-ai/vibethis/server/recipe-core v0.0.0
	github.com/divisive-ai/vibethis/server/ticket v0.0.0-00010101000000-000000000000
	github.com/fsnotify/fsnotify v1.7.0
	github.com/google/cel-go v0.26.0
	github.com/google/uuid v1.6.0
	github.com/invopop/jsonschema v0.13.0
	github.com/stretchr/testify v1.11.1
	go.temporal.io/api v1.49.1
	go.temporal.io/sdk v1.35.0
	go.uber.org/zap v1.27.0
	gopkg.in/yaml.v3 v3.0.1
	gorm.io/gorm v1.30.0
)

require (
	cel.dev/expr v0.24.0 // indirect
	github.com/antlr4-go/antlr/v4 v4.13.0 // indirect
	github.com/bahlo/generic-list-go v0.2.0 // indirect
	github.com/buger/jsonparser v1.1.1 // indirect
	github.com/divisive-ai/vibethis/server/core v0.0.0 // indirect
	github.com/facebookgo/clock v0.0.0-20150410010913-600d898af40a // indirect
	github.com/fatih/structs v1.1.0 // indirect
	github.com/gabriel-vasile/mimetype v1.4.3 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-playground/locales v0.14.1 // indirect
	github.com/go-playground/universal-translator v0.18.1 // indirect
	github.com/go-playground/validator/v10 v10.19.0 // indirect
	github.com/goccy/go-yaml v1.18.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/mock v1.6.0 // indirect
	github.com/grpc-ecosystem/go-grpc-middleware v1.4.0 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.27.2 // indirect
	github.com/imdario/mergo v0.3.16 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20231201235250-de7065d80cb9 // indirect
	github.com/jackc/pgx/v5 v5.5.5 // indirect
	github.com/jackc/puddle/v2 v2.2.1 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.1.5 // indirect
	github.com/leodido/go-urn v1.4.0 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/mattn/go-sqlite3 v1.14.22 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/nexus-rpc/sdk-go v0.3.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/robfig/cron v1.2.0 // indirect
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.2 // indirect
	github.com/stoewer/go-strcase v1.2.0 // indirect
	github.com/stretchr/objx v0.5.2 // indirect
	github.com/swaggest/jsonschema-go v0.3.78 // indirect
	github.com/swaggest/refl v1.4.0 // indirect
	github.com/wk8/go-ordered-map/v2 v2.1.9-0.20240815153524-6ea36470d1bd // indirect
	go.opentelemetry.io/auto/sdk v1.1.0 // indirect
	go.opentelemetry.io/otel v1.38.0 // indirect
	go.opentelemetry.io/otel/metric v1.38.0 // indirect
	go.opentelemetry.io/otel/trace v1.38.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	golang.org/x/crypto v0.41.0 // indirect
	golang.org/x/exp v0.0.0-20230515195305-f3d0a9c9a5cc // indirect
	golang.org/x/net v0.43.0 // indirect
	golang.org/x/sync v0.16.0 // indirect
	golang.org/x/sys v0.35.0 // indirect
	golang.org/x/text v0.28.0 // indirect
	golang.org/x/time v0.12.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20250825161204-c5933d9347a5 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250825161204-c5933d9347a5 // indirect
	google.golang.org/grpc v1.75.0 // indirect
	google.golang.org/protobuf v1.36.8 // indirect
	gorm.io/driver/postgres v1.5.11 // indirect
	gorm.io/driver/sqlite v1.5.7 // indirect
	gorm.io/plugin/optimisticlock v1.3.3 // indirect
)

replace github.com/divisive-ai/vibethis/server/recipe-core => ../recipe-core

replace github.com/divisive-ai/vibethis/server/git => ../git

replace github.com/divisive-ai/vibethis/server/core => ../core

replace github.com/divisive-ai/vibethis/server/llm => ../llm

replace github.com/divisive-ai/vibethis/server/ticket => ../ticket

replace github.com/divisive-ai/vibethis/server/container => ../container

replace gorm.io/plugin/optimisticlock => github.com/go-gorm/optimisticlock v1.1.3
