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
	vibethis/storage v0.0.0-00010101000000-000000000000
)

require (
	github.com/boltdb/bolt v1.3.1 // indirect
	golang.org/x/sys v0.33.0 // indirect
)

replace (
	vibethis/container => ../container
	vibethis/core => ../core
	vibethis/files => ../files
	vibethis/git => ../git
	vibethis/graph => ../graph
	vibethis/storage => ../storage
)
