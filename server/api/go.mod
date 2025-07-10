module vibethis/api

go 1.23.0

toolchain go1.24.1

require (
	github.com/gorilla/mux v1.8.0
	vibethis/container v0.0.0
	vibethis/core v0.0.0
	vibethis/files v0.0.0
	vibethis/git v0.0.0
	vibethis/graph v0.0.0-00010101000000-000000000000
	vibethis/openapi v0.0.0
	vibethis/storage v0.0.0-00010101000000-000000000000
)

require (
	github.com/apapsch/go-jsonmerge/v2 v2.0.0 // indirect
	github.com/boltdb/bolt v1.3.1 // indirect
	github.com/getkin/kin-openapi v0.122.0 // indirect
	github.com/go-openapi/jsonpointer v0.19.6 // indirect
	github.com/go-openapi/swag v0.22.4 // indirect
	github.com/google/uuid v1.5.0 // indirect
	github.com/invopop/yaml v0.2.0 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/mohae/deepcopy v0.0.0-20170929034955-c48cc78d4826 // indirect
	github.com/oapi-codegen/runtime v1.1.1 // indirect
	github.com/perimeterx/marshmallow v1.1.5 // indirect
	golang.org/x/sys v0.33.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace (
	vibethis/container => ../container
	vibethis/core => ../core
	vibethis/files => ../files
	vibethis/git => ../git
	vibethis/graph => ../graph
	vibethis/openapi => ../openapi
	vibethis/storage => ../storage
)
