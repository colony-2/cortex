package rwshim

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