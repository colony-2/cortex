# Moon Commands Reference

## Installation
Moon is installed at `~/.moon/bin/moon`. Add it to your PATH:
```bash
export PATH="$HOME/.moon/bin:$PATH"
```

## Project Structure
- Go modules: `go-core`, `go-storage`, `go-graph`, `go-files`, `go-git`, `go-container`, `go-web`, `vibethis`
- Web modules: `web-app`, `web-shared`, `web-changes`, `web-config`, `web-files`, `web-flowchart`

## Common Commands

### Build Commands
```bash
# Build specific project
moon run vibethis:build
moon run web-app:build

# Build with dependencies
moon run vibethis:build-prod

# Build all projects
moon run :build --all
```

### Test Commands
```bash
# Test specific project
moon run go-core:test
moon run web-app:test

# Test all projects
moon run :test --all

# Test by language
moon run :test --query "language=[typescript]"
moon run :test --query "language=[go]"
```

### Development Commands
```bash
# Run dev servers
moon run web-app:serve      # Frontend on port 5173
moon run vibethis:serve     # Backend on port 8080

# Run both in parallel (from separate terminals)
moon run web-app:serve &
moon run vibethis:serve
```

### E2E Tests
```bash
moon run web-app:e2e
moon run web-files:e2e
```

### Other Commands
```bash
# View project graph
moon project-graph

# Query projects
moon query projects
moon query tasks

# Check project info
moon project web-app
moon project vibethis

# Clean workspace
moon clean

# Setup toolchain
moon setup
```

## Task Dependencies

The `vibethis:build-prod` task automatically:
1. Builds all web dependencies (`web-shared`, `web-changes`, etc.)
2. Builds the `web-app`
3. Copies frontend dist to server static directory
4. Builds the Go binary with embedded assets

## Tips

1. Moon caches task outputs - use `moon clean` if you need fresh builds
2. Use `--log debug` for detailed output
3. Use `--concurrency` to control parallel execution
4. Tasks run from their project directory automatically
5. Environment variables can be accessed with `$MOON_WORKSPACE_ROOT`