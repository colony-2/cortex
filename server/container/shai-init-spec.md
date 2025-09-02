# Shai Init Command Specification

## Overview
`shai init` creates a new `.devcontainer/devcontainer.json` configuration file in the current directory, providing a quick way to bootstrap a devcontainer environment for use with `shai`. The command generates a sensible default configuration with optional customization of base image and features.

## Command Interface

### Usage
```bash
shai init [flags]
```

### Examples
```bash
# Create default .devcontainer/devcontainer.json
shai init

# Specify custom base image
shai init --image golang:1.24-bookworm

# Add common development features
shai init --image node:20 --feature git --feature github-cli --feature docker-in-docker

# Force overwrite existing configuration (creates backup)
shai init --force

# Create minimal configuration without any features
shai init --minimal

# Use a preset template
shai init --preset go
shai init --preset node
shai init --preset python
shai init --preset rust
shai init --preset java
shai init --preset dotnet
shai init --preset ruby
shai init --preset php
shai init --preset data-science
shai init --preset devops
shai init --preset full-stack
```

### Flags
```
--image string       Base Docker image (default: "mcr.microsoft.com/devcontainers/base:ubuntu")
--feature strings    DevContainer features to include (can be specified multiple times)
--force             Overwrite existing devcontainer.json (creates backup first)
--minimal           Create minimal configuration without default features
--preset string     Use a preset template (go, node, python, rust, java, dotnet, ruby, php, data-science, devops, full-stack)
--name string       Container name (default: based on directory name)
--no-mounts         Skip adding default mount configurations
--list-features     List all available features and exit
--list-presets      List all available presets with descriptions and exit
```

## Directory Structure

### Standard Location
Creates configuration at `.devcontainer/devcontainer.json`:
```
project/
├── .devcontainer/
│   └── devcontainer.json    # Created by shai init
├── src/
└── README.md
```

### File Creation Rules
1. Creates `.devcontainer` directory if it doesn't exist
2. Fails if `.devcontainer/devcontainer.json` already exists (unless `--force`)
3. With `--force`: backs up existing file to `.devcontainer/devcontainer.json.backup.<timestamp>`
   - Timestamp format: `20240102-150405` (YYYYMMDD-HHMMSS)
   - Example: `devcontainer.json.backup.20240102-150405`
4. Also checks for `.devcontainer.json` in project root and warns if found
5. Sets appropriate file permissions (0644 for file, 0755 for directory)

## Default Configuration

### Base Template
```json
{
  "name": "${localWorkspaceFolderBasename}",
  "image": "mcr.microsoft.com/devcontainers/base:ubuntu",
  "features": {
    "ghcr.io/devcontainers/features/common-utils:2": {
      "installZsh": true,
      "configureZshAsDefaultShell": true,
      "username": "vscode",
      "userUid": "1000",
      "userGid": "1000"
    }
  },
  "workspaceFolder": "/workspace",
  "mounts": [
    "source=${localEnv:HOME}/.ssh,target=/home/vscode/.ssh,type=bind,readonly",
    "source=${localEnv:HOME}/.gitconfig,target=/home/vscode/.gitconfig,type=bind,readonly"
  ],
  "remoteUser": "vscode",
  "customizations": {
    "vscode": {
      "settings": {},
      "extensions": []
    }
  }
}
```

### With Custom Image
When `--image` is specified:
```json
{
  "name": "${localWorkspaceFolderBasename}",
  "image": "golang:1.21-bookworm",  // User-specified image
  "features": {
    // Features adjusted based on image
  },
  // ... rest of configuration
}
```

## Feature Management

### Available Features
Common features that can be added via `--feature`:

**Version Control & CI/CD:**
- `git` → `ghcr.io/devcontainers/features/git:1`
- `github-cli` → `ghcr.io/devcontainers/features/github-cli:1`
- `gitlab-cli` → `ghcr.io/devcontainers-contrib/features/gitlab-cli:1`
- `git-lfs` → `ghcr.io/devcontainers/features/git-lfs:1`

**Container & Orchestration:**
- `docker-in-docker` → `ghcr.io/devcontainers/features/docker-in-docker:2`
- `docker-outside` → `ghcr.io/devcontainers/features/docker-outside-of-docker:1`
- `kubectl` → `ghcr.io/devcontainers/features/kubectl-helm-minikube:1`
- `kind` → `ghcr.io/devcontainers-contrib/features/kind:1`
- `k9s` → `ghcr.io/devcontainers-contrib/features/k9s:1`
- `podman` → `ghcr.io/devcontainers-contrib/features/podman:1`

**Cloud Providers:**
- `aws-cli` → `ghcr.io/devcontainers/features/aws-cli:1`
- `azure-cli` → `ghcr.io/devcontainers/features/azure-cli:1`
- `gcloud` → `ghcr.io/devcontainers-contrib/features/gcloud-cli:1`
- `doctl` → `ghcr.io/devcontainers-contrib/features/doctl:1`
- `terraform` → `ghcr.io/devcontainers/features/terraform:1`
- `pulumi` → `ghcr.io/devcontainers-contrib/features/pulumi:1`

**Programming Languages:**
- `python` → `ghcr.io/devcontainers/features/python:1`
- `node` → `ghcr.io/devcontainers/features/node:1`
- `go` → `ghcr.io/devcontainers/features/go:1`
- `rust` → `ghcr.io/devcontainers/features/rust:1`
- `java` → `ghcr.io/devcontainers/features/java:1`
- `dotnet` → `ghcr.io/devcontainers/features/dotnet:1`
- `ruby` → `ghcr.io/devcontainers/features/ruby:1`
- `php` → `ghcr.io/devcontainers/features/php:1`
- `deno` → `ghcr.io/devcontainers-contrib/features/deno:1`
- `bun` → `ghcr.io/devcontainers-contrib/features/bun:1`
- `zig` → `ghcr.io/devcontainers-contrib/features/zig:1`

**Databases & Tools:**
- `postgres-client` → `ghcr.io/devcontainers-contrib/features/postgres-client:1`
- `mysql-client` → `ghcr.io/devcontainers-contrib/features/mysql-client:1`
- `mongodb-cli` → `ghcr.io/devcontainers-contrib/features/mongosh:1`
- `redis-cli` → `ghcr.io/devcontainers-contrib/features/redis-cli:1`
- `sqlite` → `ghcr.io/devcontainers-contrib/features/sqlite:1`

**Development Tools:**
- `vscode-server` → `ghcr.io/devcontainers-contrib/features/code-server:1`
- `neovim` → `ghcr.io/devcontainers-contrib/features/neovim:1`
- `vim` → `ghcr.io/devcontainers-contrib/features/vim:1`
- `tmux` → `ghcr.io/devcontainers-contrib/features/tmux:1`
- `fzf` → `ghcr.io/devcontainers-contrib/features/fzf:1`
- `ripgrep` → `ghcr.io/devcontainers-contrib/features/ripgrep:1`
- `bat` → `ghcr.io/devcontainers-contrib/features/bat:1`
- `exa` → `ghcr.io/devcontainers-contrib/features/exa:1`
- `zsh` → `ghcr.io/devcontainers-contrib/features/zsh:1`
- `fish` → `ghcr.io/devcontainers-contrib/features/fish:1`
- `starship` → `ghcr.io/devcontainers-contrib/features/starship:1`

**Build & Package Tools:**
- `make` → `ghcr.io/devcontainers-contrib/features/make:1`
- `cmake` → `ghcr.io/devcontainers-contrib/features/cmake:1`
- `gradle` → `ghcr.io/devcontainers-contrib/features/gradle:1`
- `maven` → `ghcr.io/devcontainers-contrib/features/maven:1`
- `bazel` → `ghcr.io/devcontainers-contrib/features/bazel:1`
- `meson` → `ghcr.io/devcontainers-contrib/features/meson:1`

**Security & Monitoring:**
- `trivy` → `ghcr.io/devcontainers-contrib/features/trivy:1`
- `hadolint` → `ghcr.io/devcontainers-contrib/features/hadolint:1`
- `cosign` → `ghcr.io/devcontainers-contrib/features/cosign:1`
- `syft` → `ghcr.io/devcontainers-contrib/features/syft:1`
- `grype` → `ghcr.io/devcontainers-contrib/features/grype:1`

### Feature Resolution
```go
// Short name to full feature ID mapping
var featureMap = map[string]string{
    // Version Control
    "git":             "ghcr.io/devcontainers/features/git:1",
    "github-cli":      "ghcr.io/devcontainers/features/github-cli:1",
    "gitlab-cli":      "ghcr.io/devcontainers-contrib/features/gitlab-cli:1",
    
    // Container Tools
    "docker-in-docker": "ghcr.io/devcontainers/features/docker-in-docker:2",
    "docker-outside":   "ghcr.io/devcontainers/features/docker-outside-of-docker:1",
    "kubectl":          "ghcr.io/devcontainers/features/kubectl-helm-minikube:1",
    
    // Languages
    "python":          "ghcr.io/devcontainers/features/python:1",
    "node":            "ghcr.io/devcontainers/features/node:1",
    "go":              "ghcr.io/devcontainers/features/go:1",
    "rust":            "ghcr.io/devcontainers/features/rust:1",
    // ... etc
}
```

## Preset Templates

### Go Preset
```bash
shai init --preset go
```
Generates:
```json
{
  "name": "${localWorkspaceFolderBasename}",
  "image": "golang:1.21-bookworm",
  "features": {
    "ghcr.io/devcontainers/features/common-utils:2": {},
    "ghcr.io/devcontainers/features/git:1": {}
  },
  "workspaceFolder": "/workspace",
  "mounts": [
    "source=${localEnv:HOME}/.ssh,target=/home/vscode/.ssh,type=bind,readonly",
    "source=${localEnv:HOME}/.gitconfig,target=/home/vscode/.gitconfig,type=bind,readonly",
    "source=${localEnv:HOME}/.cache/go-build,target=/go/pkg,type=bind",
    "source=${localEnv:HOME}/.cache/go-mod,target=/go/mod,type=bind"
  ],
  "remoteUser": "vscode",
  "customizations": {
    "vscode": {
      "extensions": ["golang.go"]
    }
  },
  "postCreateCommand": "go version && go mod download"
}
```

### Node Preset
```bash
shai init --preset node
```
Generates:
```json
{
  "name": "${localWorkspaceFolderBasename}",
  "image": "node:20-bookworm",
  "features": {
    "ghcr.io/devcontainers/features/common-utils:2": {},
    "ghcr.io/devcontainers/features/git:1": {}
  },
  "workspaceFolder": "/workspace",
  "mounts": [
    "source=${localEnv:HOME}/.ssh,target=/home/node/.ssh,type=bind,readonly",
    "source=${localEnv:HOME}/.gitconfig,target=/home/node/.gitconfig,type=bind,readonly",
    "source=${localEnv:HOME}/.npm,target=/home/node/.npm,type=bind"
  ],
  "remoteUser": "node",
  "customizations": {
    "vscode": {
      "extensions": ["dbaeumer.vscode-eslint", "esbenp.prettier-vscode"]
    }
  },
  "postCreateCommand": "npm install"
}
```

### Python Preset
```bash
shai init --preset python
```
Generates:
```json
{
  "name": "${localWorkspaceFolderBasename}",
  "image": "python:3.11-bookworm",
  "features": {
    "ghcr.io/devcontainers/features/common-utils:2": {},
    "ghcr.io/devcontainers/features/git:1": {}
  },
  "workspaceFolder": "/workspace",
  "mounts": [
    "source=${localEnv:HOME}/.ssh,target=/home/vscode/.ssh,type=bind,readonly",
    "source=${localEnv:HOME}/.gitconfig,target=/home/vscode/.gitconfig,type=bind,readonly",
    "source=${localEnv:HOME}/.cache/pip,target=/home/vscode/.cache/pip,type=bind"
  ],
  "remoteUser": "vscode",
  "customizations": {
    "vscode": {
      "extensions": ["ms-python.python", "ms-python.vscode-pylance"]
    }
  },
  "postCreateCommand": "pip install --upgrade pip && if [ -f requirements.txt ]; then pip install -r requirements.txt; fi"
}
```

### Rust Preset
```bash
shai init --preset rust
```
Generates:
```json
{
  "name": "${localWorkspaceFolderBasename}",
  "image": "rust:1-bookworm",
  "features": {
    "ghcr.io/devcontainers/features/common-utils:2": {},
    "ghcr.io/devcontainers/features/git:1": {},
    "ghcr.io/devcontainers/features/rust:1": {
      "version": "stable",
      "profile": "default"
    }
  },
  "workspaceFolder": "/workspace",
  "mounts": [
    "source=${localEnv:HOME}/.ssh,target=/home/vscode/.ssh,type=bind,readonly",
    "source=${localEnv:HOME}/.gitconfig,target=/home/vscode/.gitconfig,type=bind,readonly",
    "source=${localEnv:HOME}/.cargo/registry,target=/usr/local/cargo/registry,type=bind",
    "source=${localEnv:HOME}/.cargo/git,target=/usr/local/cargo/git,type=bind"
  ],
  "remoteUser": "vscode",
  "customizations": {
    "vscode": {
      "extensions": ["rust-lang.rust-analyzer", "tamasfe.even-better-toml"]
    }
  },
  "postCreateCommand": "rustc --version && cargo --version"
}
```

### Java Preset
```bash
shai init --preset java
```
Generates:
```json
{
  "name": "${localWorkspaceFolderBasename}",
  "image": "mcr.microsoft.com/devcontainers/java:17-bookworm",
  "features": {
    "ghcr.io/devcontainers/features/java:1": {
      "version": "17",
      "installMaven": true,
      "installGradle": true
    },
    "ghcr.io/devcontainers/features/git:1": {}
  },
  "workspaceFolder": "/workspace",
  "mounts": [
    "source=${localEnv:HOME}/.ssh,target=/home/vscode/.ssh,type=bind,readonly",
    "source=${localEnv:HOME}/.gitconfig,target=/home/vscode/.gitconfig,type=bind,readonly",
    "source=${localEnv:HOME}/.m2,target=/home/vscode/.m2,type=bind",
    "source=${localEnv:HOME}/.gradle,target=/home/vscode/.gradle,type=bind"
  ],
  "remoteUser": "vscode",
  "customizations": {
    "vscode": {
      "extensions": ["vscjava.vscode-java-pack", "redhat.java"]
    }
  },
  "postCreateCommand": "java --version"
}
```

### Ruby Preset
```bash
shai init --preset ruby
```
Generates:
```json
{
  "name": "${localWorkspaceFolderBasename}",
  "image": "ruby:3.2-bookworm",
  "features": {
    "ghcr.io/devcontainers/features/common-utils:2": {},
    "ghcr.io/devcontainers/features/git:1": {}
  },
  "workspaceFolder": "/workspace",
  "mounts": [
    "source=${localEnv:HOME}/.ssh,target=/home/vscode/.ssh,type=bind,readonly",
    "source=${localEnv:HOME}/.gitconfig,target=/home/vscode/.gitconfig,type=bind,readonly",
    "source=${localEnv:HOME}/.bundle,target=/home/vscode/.bundle,type=bind",
    "source=${localEnv:HOME}/.gem,target=/home/vscode/.gem,type=bind"
  ],
  "remoteUser": "vscode",
  "customizations": {
    "vscode": {
      "extensions": ["rebornix.ruby", "castwide.solargraph"]
    }
  },
  "postCreateCommand": "ruby --version && gem install bundler && bundle install || true"
}
```

### Data Science Preset
```bash
shai init --preset data-science
```
Generates:
```json
{
  "name": "${localWorkspaceFolderBasename}",
  "image": "mcr.microsoft.com/devcontainers/anaconda:3",
  "features": {
    "ghcr.io/devcontainers/features/git:1": {},
    "ghcr.io/devcontainers/features/github-cli:1": {},
    "ghcr.io/devcontainers-contrib/features/jupyterlab:1": {}
  },
  "workspaceFolder": "/workspace",
  "mounts": [
    "source=${localEnv:HOME}/.ssh,target=/home/vscode/.ssh,type=bind,readonly",
    "source=${localEnv:HOME}/.gitconfig,target=/home/vscode/.gitconfig,type=bind,readonly",
    "source=${localEnv:HOME}/.cache/pip,target=/home/vscode/.cache/pip,type=bind",
    "source=${localEnv:HOME}/.jupyter,target=/home/vscode/.jupyter,type=bind",
    "source=${localEnv:HOME}/.ipython,target=/home/vscode/.ipython,type=bind"
  ],
  "remoteUser": "vscode",
  "customizations": {
    "vscode": {
      "extensions": [
        "ms-python.python",
        "ms-python.vscode-pylance",
        "ms-toolsai.jupyter",
        "ms-toolsai.jupyter-keymap",
        "ms-toolsai.jupyter-renderers"
      ]
    }
  },
  "postCreateCommand": "pip install pandas numpy matplotlib seaborn scikit-learn jupyter",
  "forwardPorts": [8888]
}
```

### DevOps Preset
```bash
shai init --preset devops
```
Generates:
```json
{
  "name": "${localWorkspaceFolderBasename}",
  "image": "mcr.microsoft.com/devcontainers/base:ubuntu",
  "features": {
    "ghcr.io/devcontainers/features/common-utils:2": {},
    "ghcr.io/devcontainers/features/git:1": {},
    "ghcr.io/devcontainers/features/docker-in-docker:2": {},
    "ghcr.io/devcontainers/features/kubectl-helm-minikube:1": {},
    "ghcr.io/devcontainers/features/terraform:1": {},
    "ghcr.io/devcontainers/features/aws-cli:1": {},
    "ghcr.io/devcontainers/features/azure-cli:1": {},
    "ghcr.io/devcontainers/features/github-cli:1": {}
  },
  "workspaceFolder": "/workspace",
  "mounts": [
    "source=${localEnv:HOME}/.ssh,target=/home/vscode/.ssh,type=bind,readonly",
    "source=${localEnv:HOME}/.gitconfig,target=/home/vscode/.gitconfig,type=bind,readonly",
    "source=${localEnv:HOME}/.aws,target=/home/vscode/.aws,type=bind,readonly",
    "source=${localEnv:HOME}/.azure,target=/home/vscode/.azure,type=bind,readonly",
    "source=${localEnv:HOME}/.kube,target=/home/vscode/.kube,type=bind"
  ],
  "remoteUser": "vscode",
  "customizations": {
    "vscode": {
      "extensions": [
        "hashicorp.terraform",
        "ms-kubernetes-tools.vscode-kubernetes-tools",
        "ms-azuretools.vscode-docker"
      ]
    }
  }
}
```

### Full-Stack Preset
```bash
shai init --preset full-stack
```
Generates:
```json
{
  "name": "${localWorkspaceFolderBasename}",
  "image": "mcr.microsoft.com/devcontainers/javascript-node:20",
  "features": {
    "ghcr.io/devcontainers/features/common-utils:2": {},
    "ghcr.io/devcontainers/features/git:1": {},
    "ghcr.io/devcontainers/features/github-cli:1": {},
    "ghcr.io/devcontainers/features/python:1": {},
    "ghcr.io/devcontainers-contrib/features/postgres-client:1": {},
    "ghcr.io/devcontainers-contrib/features/redis-cli:1": {}
  },
  "workspaceFolder": "/workspace",
  "mounts": [
    "source=${localEnv:HOME}/.ssh,target=/home/vscode/.ssh,type=bind,readonly",
    "source=${localEnv:HOME}/.gitconfig,target=/home/vscode/.gitconfig,type=bind,readonly",
    "source=${localEnv:HOME}/.npm,target=/home/vscode/.npm,type=bind",
    "source=${localEnv:HOME}/.cache/pip,target=/home/vscode/.cache/pip,type=bind"
  ],
  "remoteUser": "vscode",
  "customizations": {
    "vscode": {
      "extensions": [
        "dbaeumer.vscode-eslint",
        "esbenp.prettier-vscode",
        "prisma.prisma",
        "graphql.vscode-graphql"
      ]
    }
  },
  "postCreateCommand": "npm install && pip install -r requirements.txt || true",
  "forwardPorts": [3000, 5000, 5432, 6379]
}
```

## Implementation Details

### Command Structure
```go
// cmd/shai/init.go
package main

import (
    "encoding/json"
    "flag"
    "fmt"
    "os"
    "path/filepath"
)

type InitConfig struct {
    Image     string
    Features  []string
    Force     bool
    Minimal   bool
    Preset    string
    Name      string
    NoMounts  bool
}

func runInit(config InitConfig) error {
    // Check for existing configuration
    devcontainerPath := filepath.Join(".devcontainer", "devcontainer.json")
    if _, err := os.Stat(devcontainerPath); err == nil {
        if !config.Force {
            return fmt.Errorf("devcontainer.json already exists. Use --force to overwrite")
        }
        
        // Create backup with timestamp
        timestamp := time.Now().Format("20060102-150405")
        backupPath := fmt.Sprintf("%s.backup.%s", devcontainerPath, timestamp)
        
        // Read existing file
        data, err := os.ReadFile(devcontainerPath)
        if err != nil {
            return fmt.Errorf("failed to read existing file for backup: %w", err)
        }
        
        // Write backup
        if err := os.WriteFile(backupPath, data, 0644); err != nil {
            return fmt.Errorf("failed to create backup: %w", err)
        }
        
        fmt.Printf("✓ Backed up existing configuration to %s\n", backupPath)
    }
    
    // Check for alternate location
    if _, err := os.Stat(".devcontainer.json"); err == nil {
        fmt.Fprintf(os.Stderr, "Warning: .devcontainer.json exists in project root\n")
    }
    
    // Create .devcontainer directory
    if err := os.MkdirAll(".devcontainer", 0755); err != nil {
        return fmt.Errorf("failed to create .devcontainer directory: %w", err)
    }
    
    // Generate configuration
    devConfig := generateConfig(config)
    
    // Write configuration
    data, err := json.MarshalIndent(devConfig, "", "  ")
    if err != nil {
        return fmt.Errorf("failed to marshal configuration: %w", err)
    }
    
    if err := os.WriteFile(devcontainerPath, data, 0644); err != nil {
        return fmt.Errorf("failed to write devcontainer.json: %w", err)
    }
    
    fmt.Printf("✓ Created %s\n", devcontainerPath)
    fmt.Printf("✓ Base image: %s\n", devConfig.Image)
    if len(devConfig.Features) > 0 {
        fmt.Printf("✓ Features: %d configured\n", len(devConfig.Features))
    }
    fmt.Println("\nYou can now run: shai -rw <directory>")
    
    return nil
}
```

### Configuration Generation
```go
func generateConfig(config InitConfig) *DevContainer {
    if config.Preset != "" {
        return generatePresetConfig(config.Preset)
    }
    
    dc := &DevContainer{
        Name:  config.Name,
        Image: config.Image,
    }
    
    if dc.Name == "" {
        dc.Name = "${localWorkspaceFolderBasename}"
    }
    
    if dc.Image == "" {
        dc.Image = "mcr.microsoft.com/devcontainers/base:ubuntu"
    }
    
    if !config.Minimal {
        dc.Features = getDefaultFeatures()
        for _, feature := range config.Features {
            dc.Features[resolveFeature(feature)] = map[string]interface{}{}
        }
    }
    
    if !config.NoMounts {
        dc.Mounts = getDefaultMounts()
    }
    
    dc.WorkspaceFolder = "/workspace"
    dc.RemoteUser = "vscode"
    
    return dc
}
```

## Error Handling

### Error Scenarios
1. **Existing configuration**: Clear message about using `--force`
2. **Permission denied**: Suggest running with appropriate permissions
3. **Invalid preset**: List available presets
4. **Unknown feature**: Suggest similar features or list available
5. **Invalid image format**: Provide example of valid image names

### Example Error Messages
```
Error: devcontainer.json already exists at .devcontainer/devcontainer.json
Use 'shai init --force' to overwrite the existing configuration

Error: Unknown preset 'ruby'. Available presets: go, node, python, rust

Error: Unknown feature 'github'. Did you mean 'github-cli'?
Available features: git, github-cli, docker-in-docker, kubectl, ...

Error: Permission denied creating .devcontainer directory
Try running with appropriate permissions or check directory ownership
```

## User Experience

### Success Flow
```bash
$ shai init --image golang:1.21 --feature git --feature docker-in-docker
✓ Created .devcontainer/devcontainer.json
✓ Base image: golang:1.21
✓ Features: 3 configured
  - ghcr.io/devcontainers/features/common-utils:2
  - ghcr.io/devcontainers/features/git:1
  - ghcr.io/devcontainers/features/docker-in-docker:2

You can now run: shai -rw <directory>
```

### Force Overwrite Flow
```bash
$ shai init --force --preset python
✓ Backed up existing configuration to .devcontainer/devcontainer.json.backup.20240102-150405
✓ Created .devcontainer/devcontainer.json
✓ Base image: python:3.11-bookworm
✓ Features: 2 configured
  - ghcr.io/devcontainers/features/common-utils:2
  - ghcr.io/devcontainers/features/git:1

You can now run: shai -rw <directory>
```

### List Features Flow
```bash
$ shai init --list-features
Available DevContainer Features:

Version Control & CI/CD:
  git              Git version control
  github-cli       GitHub CLI (gh)
  gitlab-cli       GitLab CLI (glab)
  git-lfs          Git Large File Storage

Container & Orchestration:
  docker-in-docker Run Docker inside container
  docker-outside   Use host Docker from container
  kubectl          Kubernetes CLI with Helm
  kind             Kubernetes in Docker
  k9s              Kubernetes terminal UI
  podman           Podman container runtime

[... more categories ...]
```


## Testing Requirements

### Unit Tests
- Configuration generation with various flag combinations
- Feature resolution and validation
- Preset template generation
- Mount configuration generation

### Integration Tests
- File creation and permissions
- Overwrite behavior with --force flag
- Directory creation when missing
- Conflict detection with existing files

### E2E Tests
- Full init flow followed by `shai -rw` command
- Verify generated configuration works with Docker
- Test each preset creates working environment
- Validate mounted directories are accessible