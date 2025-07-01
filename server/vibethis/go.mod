module vibethis/vibethis

go 1.23.0

toolchain go1.24.1

require (
	github.com/spf13/cobra v1.7.0
	vibethis/api v0.0.0
	vibethis/container v0.0.0
	vibethis/files v0.0.0
	vibethis/git v0.0.0
	vibethis/graph v0.0.0
	vibethis/storage v0.0.0
)

require (
	dario.cat/mergo v1.0.0 // indirect
	github.com/Microsoft/go-winio v0.6.2 // indirect
	github.com/ProtonMail/go-crypto v1.1.6 // indirect
	github.com/boltdb/bolt v1.3.1 // indirect
	github.com/cloudflare/circl v1.6.1 // indirect
	github.com/cyphar/filepath-securejoin v0.4.1 // indirect
	github.com/emirpasic/gods v1.18.1 // indirect
	github.com/go-git/gcfg v1.5.1-0.20230307220236-3a3c6141e376 // indirect
	github.com/go-git/go-billy/v5 v5.6.2 // indirect
	github.com/go-git/go-git/v5 v5.16.2 // indirect
	github.com/golang/groupcache v0.0.0-20241129210726-2c02b8208cf8 // indirect
	github.com/gorilla/mux v1.8.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/jbenet/go-context v0.0.0-20150711004518-d14ea06fba99 // indirect
	github.com/kevinburke/ssh_config v1.2.0 // indirect
	github.com/pjbgf/sha1cd v0.3.2 // indirect
	github.com/sergi/go-diff v1.3.2-0.20230802210424-5b0b94c5c0d3 // indirect
	github.com/skeema/knownhosts v1.3.1 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/xanzy/ssh-agent v0.3.3 // indirect
	golang.org/x/crypto v0.37.0 // indirect
	golang.org/x/net v0.39.0 // indirect
	golang.org/x/sys v0.33.0 // indirect
	gopkg.in/warnings.v0 v0.1.2 // indirect
	vibethis/core v0.0.0 // indirect
)

replace (
	vibethis/api => ../api
	vibethis/container => ../container
	vibethis/core => ../core
	vibethis/files => ../files
	vibethis/git => ../git
	vibethis/graph => ../graph
	vibethis/storage => ../storage
)
