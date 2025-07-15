package rwshimgo

// Common policy implementations

// AllowAll is a policy that allows all operations
func AllowAll(req Request) Response {
	return Response{Allow: true}
}

// DenyAll is a policy that denies all operations
func DenyAll(req Request) Response {
	return Response{Allow: false}
}

// DenyWrites is a policy that allows reads but denies writes
func DenyWrites(req Request) Response {
	return Response{Allow: req.Operation == OpRead}
}

// DenyReads is a policy that allows writes but denies reads
func DenyReads(req Request) Response {
	return Response{Allow: req.Operation == OpWrite}
}

// PolicyBuilder helps create complex policies
type PolicyBuilder struct {
	rules []PolicyRule
}

// PolicyRule represents a single policy rule
type PolicyRule struct {
	Match func(Request) bool
	Allow bool
}

// NewPolicyBuilder creates a new policy builder
func NewPolicyBuilder() *PolicyBuilder {
	return &PolicyBuilder{
		rules: make([]PolicyRule, 0),
	}
}

// AllowRead adds a rule to allow read operations matching the condition
func (pb *PolicyBuilder) AllowRead(match func(Request) bool) *PolicyBuilder {
	pb.rules = append(pb.rules, PolicyRule{
		Match: func(req Request) bool {
			return req.Operation == OpRead && match(req)
		},
		Allow: true,
	})
	return pb
}

// AllowWrite adds a rule to allow write operations matching the condition
func (pb *PolicyBuilder) AllowWrite(match func(Request) bool) *PolicyBuilder {
	pb.rules = append(pb.rules, PolicyRule{
		Match: func(req Request) bool {
			return req.Operation == OpWrite && match(req)
		},
		Allow: true,
	})
	return pb
}

// DenyRead adds a rule to deny read operations matching the condition
func (pb *PolicyBuilder) DenyRead(match func(Request) bool) *PolicyBuilder {
	pb.rules = append(pb.rules, PolicyRule{
		Match: func(req Request) bool {
			return req.Operation == OpRead && match(req)
		},
		Allow: false,
	})
	return pb
}

// DenyWrite adds a rule to deny write operations matching the condition
func (pb *PolicyBuilder) DenyWrite(match func(Request) bool) *PolicyBuilder {
	pb.rules = append(pb.rules, PolicyRule{
		Match: func(req Request) bool {
			return req.Operation == OpWrite && match(req)
		},
		Allow: false,
	})
	return pb
}

// Default sets the default behavior when no rules match
func (pb *PolicyBuilder) Default(allow bool) *PolicyBuilder {
	pb.rules = append(pb.rules, PolicyRule{
		Match: func(req Request) bool { return true },
		Allow: allow,
	})
	return pb
}

// Build creates a PolicyFunc from the configured rules
func (pb *PolicyBuilder) Build() PolicyFunc {
	// Make a copy of rules to avoid mutation
	rules := make([]PolicyRule, len(pb.rules))
	copy(rules, pb.rules)

	return func(req Request) Response {
		// Rules are evaluated in order, first match wins
		for _, rule := range rules {
			if rule.Match(req) {
				return Response{Allow: rule.Allow}
			}
		}
		// Default to deny if no rules match
		return Response{Allow: false}
	}
}

// Common matcher functions

// MatchFilename returns a matcher that checks if the filename matches
func MatchFilename(filename string) func(Request) bool {
	return func(req Request) bool {
		return req.Filename == filename
	}
}

// MatchFilenamePrefix returns a matcher that checks if the filename has a prefix
func MatchFilenamePrefix(prefix string) func(Request) bool {
	return func(req Request) bool {
		return len(req.Filename) >= len(prefix) && req.Filename[:len(prefix)] == prefix
	}
}

// MatchFilenameSuffix returns a matcher that checks if the filename has a suffix
func MatchFilenameSuffix(suffix string) func(Request) bool {
	return func(req Request) bool {
		return len(req.Filename) >= len(suffix) && 
			req.Filename[len(req.Filename)-len(suffix):] == suffix
	}
}

// MatchFD returns a matcher that checks if the file descriptor matches
func MatchFD(fd int) func(Request) bool {
	return func(req Request) bool {
		return req.FD == fd
	}
}

// MatchStdStreams returns a matcher for stdin/stdout/stderr
func MatchStdStreams() func(Request) bool {
	return func(req Request) bool {
		return req.FD >= 0 && req.FD <= 2 ||
			req.Filename == "stdin" ||
			req.Filename == "stdout" ||
			req.Filename == "stderr"
	}
}

// MatchLargeOperations returns a matcher for operations over a size threshold
func MatchLargeOperations(threshold int64) func(Request) bool {
	return func(req Request) bool {
		return req.Size > threshold
	}
}