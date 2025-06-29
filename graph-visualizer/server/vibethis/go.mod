module vibethis/vibethis

go 1.23.0

toolchain go1.24.1

require (
	github.com/spf13/cobra v1.7.0
	vibethis/container v0.0.0
	vibethis/files v0.0.0
	vibethis/git v0.0.0
	vibethis/graph v0.0.0
	vibethis/storage v0.0.0
	vibethis/web v0.0.0
)

require (
	github.com/boltdb/bolt v1.3.1 // indirect
	github.com/gorilla/mux v1.8.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	golang.org/x/sys v0.33.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	vibethis/core v0.0.0 // indirect
)

replace (
	vibethis/container => ../container
	vibethis/core => ../core
	vibethis/files => ../files
	vibethis/git => ../git
	vibethis/graph => ../graph
	vibethis/storage => ../storage
	vibethis/web => ../web
)
