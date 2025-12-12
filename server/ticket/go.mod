module github.com/divisive-ai/vibethis/server/ticket

go 1.24.1

require (
	github.com/colony-2/swf-go v0.0.0
	github.com/divisive-ai/vibethis/server/cell v0.0.0
	github.com/divisive-ai/vibethis/server/core v0.0.0
	github.com/divisive-ai/vibethis/server/project v0.0.0
	github.com/divisive-ai/vibethis/server/recipe-core v0.0.0
	github.com/fergusstrange/embedded-postgres v1.32.0
	github.com/go-playground/validator/v10 v10.19.0
	github.com/imdario/mergo v0.3.16
	github.com/segmentio/ksuid v1.0.4
	github.com/stretchr/testify v1.11.1
	go.opentelemetry.io/otel v1.37.0
	go.opentelemetry.io/otel/metric v1.37.0
	gorm.io/driver/postgres v1.5.11
	gorm.io/driver/sqlite v1.5.7
	gorm.io/gorm v1.30.0
	gorm.io/plugin/optimisticlock v1.3.3
)

require (
	github.com/apapsch/go-jsonmerge/v2 v2.0.0 // indirect
	github.com/bahlo/generic-list-go v0.2.0 // indirect
	github.com/buger/jsonparser v1.1.1 // indirect
	github.com/colony-2/pgwf-go v0.0.0-20251126023645-3cf5a829bccb // indirect
	github.com/colony-2/strata/strata-go v0.0.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/divisive-ai/vibethis/server/graph v0.0.0-00010101000000-000000000000 // indirect
	github.com/fatih/structs v1.1.0 // indirect
	github.com/gabriel-vasile/mimetype v1.4.3 // indirect
	github.com/go-chi/chi/v5 v5.0.10 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-playground/locales v0.14.1 // indirect
	github.com/go-playground/universal-translator v0.18.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/invopop/jsonschema v0.13.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20231201235250-de7065d80cb9 // indirect
	github.com/jackc/pgx/v5 v5.5.5 // indirect
	github.com/jackc/puddle/v2 v2.2.1 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.1.5 // indirect
	github.com/leodido/go-urn v1.4.0 // indirect
	github.com/lib/pq v1.10.9 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/mattn/go-sqlite3 v1.14.22 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/oapi-codegen/nullable v1.1.0 // indirect
	github.com/oapi-codegen/runtime v1.1.2 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/wk8/go-ordered-map/v2 v2.1.9-0.20240815153524-6ea36470d1bd // indirect
	github.com/xi2/xz v0.0.0-20171230120015-48954b6210f8 // indirect
	go.opentelemetry.io/auto/sdk v1.1.0 // indirect
	go.opentelemetry.io/otel/trace v1.37.0 // indirect
	golang.org/x/crypto v0.41.0 // indirect
	golang.org/x/net v0.42.0 // indirect
	golang.org/x/sync v0.16.0 // indirect
	golang.org/x/sys v0.35.0 // indirect
	golang.org/x/text v0.28.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/divisive-ai/vibethis/server/core => ../core

replace github.com/divisive-ai/vibethis/server/graph => ../graph

replace github.com/divisive-ai/vibethis/server/cell => ../cell

replace github.com/divisive-ai/vibethis/server/project => ../project

replace github.com/divisive-ai/vibethis/server/recipe-core => ../recipe-core

replace gorm.io/plugin/optimisticlock => github.com/go-gorm/optimisticlock v1.1.3

replace github.com/colony-2/swf-go v0.0.0 => ../../../swf-go

replace github.com/colony-2/strata/strata-go v0.0.0 => ../../../strata-go
