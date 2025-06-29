module vibethis/storage

go 1.23.0

toolchain go1.24.1

require (
	github.com/boltdb/bolt v1.3.1
	vibethis/core v0.0.0
)

require golang.org/x/sys v0.33.0 // indirect

replace vibethis/core => ../core
