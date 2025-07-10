# Project: Go Application

## Language & Tools
- **Language**: Go
- **Testing**: Go's built-in testing framework
- **Build**: `moon run build`
- **Test**: `moon run test`

## Development Rules

### 1. Test-Driven Development
- Write tests BEFORE implementing features
- All code must have corresponding tests
- Minimum 80% code coverage
- Run `moon run test integration` before marking any task complete

### 2. Definition of Done
A feature is ONLY complete when:
- [ ] Tests written and passing
- [ ] Code implementation complete
- [ ] All tests pass (`moon run test integration`)
- [ ] No lint errors

### 4. Project Structure
```
.
├── cmd/           # Main applications
├── internal/      # Private application code
├── pkg/           # Public libraries
├── test/          # Additional test data
└── go.mod         # Module definition
```

### 5. Testing Guidelines
- Unit tests: `*_test.go` in same package
- Use table-driven tests for multiple cases
- Mock external dependencies
- Create integration tests with integration tag to ensure coverage of end-to-end scenarios
- Test both success and error paths

### 6. Commands
```bash
# Run all tests
moon run test integration

# Run tests with coverage
moon run test -- -cover

# Run specific test
moon run test -- TestName ./...

```

## Important: No code is considered complete without passing tests!