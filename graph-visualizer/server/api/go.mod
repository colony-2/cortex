module vibethis/api

go 1.21

require (
	github.com/gorilla/mux v1.8.0
	vibethis/container v0.0.0
	vibethis/core v0.0.0
	vibethis/files v0.0.0
	vibethis/git v0.0.0
)

replace (
	vibethis/container => ../container
	vibethis/core => ../core
	vibethis/files => ../files
	vibethis/git => ../git
	vibethis/graph => ../graph
	vibethis/storage => ../storage
)
