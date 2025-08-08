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
	github.com/boltdb/bolt v1.3.1 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/spf13/pflag v1.0.6 // indirect
	golang.org/x/sys v0.33.0 // indirect
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
