package rwshim

// Operation represents a read or write operation
type Operation string

const (
	// OpRead represents a read operation
	OpRead Operation = "READ"
	// OpWrite represents a write operation
	OpWrite Operation = "WRITE"
)

// Request represents an intercepted I/O operation
type Request struct {
	Operation Operation
	FD        int
	Size      int64
	Filename  string
}

// Response represents the decision for an I/O operation
type Response struct {
	Allow bool
}

// PolicyFunc is the callback function type for deciding whether to allow operations
type PolicyFunc func(req Request) Response