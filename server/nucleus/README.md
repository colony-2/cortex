# Recipe Watcher CLI

A command-line tool that provides a managed way to run the recipe-worker package with configurable options. It monitors recipe files and dynamically executes workflows through Temporal.

## Installation

```bash
make build
```

This will create the binary at `bin/nucleus`.

## Usage

```bash
nucleus --name <worker-name> --recipes-path <path-to-recipes>
```

### Flags

- `--name` (required): Worker name/identifier for queue naming
- `--recipes-path, -r` (required): Path to recipes directory
- `--temporal-server, -t`: Temporal server address (default: "localhost:7233")
- `--namespace`: Temporal namespace (default: "default")
- `--debug, -d`: Enable debug logging (default: false)

### Examples

Basic usage:
```bash
nucleus --name my-worker --recipes-path /path/to/recipes
```

Custom configuration:
```bash
nucleus \
  --name production-worker \
  --recipes-path /path/to/recipes \
  --temporal-server temporal.example.com:7233 \
  --namespace production-ns \
  --debug
```

## Development

### Running Tests

```bash
moon run test integration
```

### Building

```bash
moon run build
```


## How It Works

The nucleus CLI wraps the recipe-worker package, which provides:

1. **Automatic Recipe Discovery**: Scans the specified directory for recipe YAML files
2. **Live Reloading**: Monitors file changes and automatically updates workers
3. **Temporal Integration**: Creates and manages Temporal workers for each recipe
4. **Dynamic Workflows**: Executes workflows based on YAML definitions

Task queues are created following the pattern: `ono-recipes-{recipe-name}`

## Requirements

- Go 1.24.1 or later
- Running Temporal server
- Access to recipe YAML files