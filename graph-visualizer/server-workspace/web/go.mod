module vibethis/web

go 1.21

require (
	vibethis/core v0.0.0
	vibethis/storage v0.0.0
	vibethis/graph v0.0.0
	vibethis/files v0.0.0
	vibethis/git v0.0.0
	vibethis/container v0.0.0
	github.com/gorilla/mux v1.8.0
	github.com/gorilla/websocket v1.5.0
)

replace (
	vibethis/core => ../core
	vibethis/storage => ../storage
	vibethis/graph => ../graph
	vibethis/files => ../files
	vibethis/git => ../git
	vibethis/container => ../container
)