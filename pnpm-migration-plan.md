# npm to pnpm Migration Plan

## Overview

Migrate the Colony2 monorepo from npm to pnpm, integrating pnpm with moon build orchestration and configuring cross-platform artifact downloads.

**Note**: This migration uses pnpm without workspace configuration, keeping the existing `file:` protocol for local dependencies. Moon handles the orchestration of multi-package installs and builds.

## Goals

1. Replace npm with pnpm as the package manager
2. Configure moon to use pnpm instead of npm
3. Set up pnpm to download native artifacts for both darwin and linux (matching current architecture - arm64 or x64)
4. Maintain current build and development workflows with `file:` protocol dependencies
5. Improve install performance and disk usage through pnpm's content-addressable storage

## Benefits of pnpm

- **Faster installs**: Symlinked dependencies from global store
- **Disk efficiency**: Single copy of each package version across all projects
- **Strict dependency resolution**: Prevents phantom dependencies
- **Supports file: protocol**: Works with existing local dependency structure
- **Cross-platform artifact caching**: Supports downloading multiple platform binaries

## Why Not Use pnpm Workspaces?

This migration avoids pnpm workspace configuration because:

1. **Moon already handles orchestration**: Moon manages the build graph and dependency ordering across packages
2. **Simpler migration**: No need to update package.json dependencies from `file:` to `workspace:` protocol
3. **Explicit lock files**: Each package has its own `pnpm-lock.yaml`, making dependencies clearer per package
4. **Flexibility**: Easier to run pnpm commands in individual packages without workspace-specific flags
5. **Existing patterns**: Keeps current dependency structure intact

Moon's task dependencies already ensure packages are built in the correct order, making workspace hoisting less critical.

## Migration Steps

### 1. Install pnpm Globally

```bash
# Install pnpm globally (if not already installed)
npm install -g pnpm@latest
```

### 2. Configure pnpm for Cross-Platform Artifacts

Create `.npmrc` at repository root (`/src/.npmrc`):

```ini
# Download artifacts for both darwin and linux platforms
# This ensures native binaries work across development (macOS) and production (Linux)
supportedArchitectures[os][]=darwin
supportedArchitectures[os][]=linux
# supportedArchitectures[cpu][]=arm64  # Uncomment if using arm64
# supportedArchitectures[cpu][]=x64    # Uncomment if using x64
```

**Note**: For CPU architecture, detect at install time or configure both:
```ini
supportedArchitectures[cpu][]=arm64
supportedArchitectures[cpu][]=x64
```

Alternative: Use pnpm config command:
```bash
pnpm config set supportedArchitectures.os "darwin,linux"
pnpm config set supportedArchitectures.cpu "arm64,x64"
```

### 3. Keep Current Dependency Structure

**No changes needed to package.json dependencies** - pnpm supports the `file:` protocol just like npm:

```json
{
  "dependencies": {
    "@colony2/shared": "file:../shared"
  }
}
```

pnpm will handle these local file dependencies without requiring workspace configuration. Each package will maintain its own `pnpm-lock.yaml` file, similar to how npm creates individual `package-lock.json` files.

**Note**: Without a workspace configuration, you'll need to run `pnpm install` in each package directory separately, or use moon's task system (which already handles this orchestration).

### 4. Update Moon Configuration

**Update `/src/.moon/toolchain.yml`:**

```yaml
node:
  version: '24.3.0'
  packageManager: 'pnpm'  # Changed from 'npm'
  pnpm:
    version: '9.15.0'  # Or latest stable version
  syncPackageManagerField: false
```

**Update `/src/.moon/tasks/node.yml`:**

Replace npm commands with pnpm equivalents:

```yaml
# No changes needed - moon automatically uses configured package manager
# Verify that tasks use package manager-agnostic commands:
tasks:
  build:
    command: 'vite build'  # Already agnostic
    inputs:
      - 'src/**/*'
      - 'package.json'
      - 'pnpm-lock.yaml'  # Changed from package-lock.json

  test:
    command: 'pnpm exec vitest run --reporter=dot'  # Use 'pnpm exec' instead of 'npx'
```

**Update project-specific moon.yml files:**

- `/src/web/kanban/moon.yml`:
  ```yaml
  tasks:
    build:
      command: 'pnpm run build'  # Changed from 'npm run build'
      inputs:
        - 'src/**'
        - 'package.json'
        - 'pnpm-lock.yaml'  # Changed from package-lock.json
        - 'tsconfig.json'
        - 'vite.config.ts'
  ```

- `/src/server/cortex/moon.yml`:
  ```yaml
  tasks:
    npm-install:  # Rename to pnpm-install
      command: 'pnpm install'
      inputs:
        - 'package.json'
        - 'pnpm-lock.yaml'

    playwright-install:
      command: 'pnpm exec playwright install chromium'  # Changed from npx
      deps:
        - '~:pnpm-install'  # Updated dependency reference
  ```

- `/src/web/app/moon.yml`:
  ```yaml
  tasks:
    integration2:
      command: 'pnpm exec playwright test --reporter=list'  # Changed from npx
  ```

- `/src/web/openapi/moon.yml`:
  ```yaml
  tasks:
    build:
      command: |
        pnpm exec openapi-typescript-codegen ...  # Changed from npx
        pnpm exec tsup
        pnpm exec tsc --emitDeclarationOnly
  ```

### 6. Migration Execution

**Step-by-step execution:**

1. **Backup current state:**
   ```bash
   git checkout -b npm-to-pnpm-migration
   ```

2. **Create pnpm configuration files:**
   ```bash
   # Create .npmrc with cross-platform config
   ```

3. **Update moon configuration:**
   ```bash
   # Edit .moon/toolchain.yml
   # Update .moon/tasks/node.yml
   # Update all project moon.yml files
   ```

4. **Remove npm artifacts:**
   ```bash
   rm -rf node_modules package-lock.json
   find . -name "node_modules" -type d -prune -exec rm -rf {} +
   find . -name "package-lock.json" -delete
   ```

5. **Install with pnpm:**
   ```bash
   # Moon will handle running pnpm install in each package
   # You can also manually install in each package if needed:
   cd web/app && pnpm install
   cd ../flowchart && pnpm install
   cd ../kanban && pnpm install
   cd ../openapi && pnpm install
   cd ../shared && pnpm install
   cd ../../server/cortex && pnpm install

   # Or let moon handle it when running tasks
   ```

6. **Verify builds:**
   ```bash
   moon run :build
   moon run :test
   ```

### 7. Verification Checklist

- [ ] .npmrc configured with supportedArchitectures for darwin and linux
- [ ] Moon toolchain.yml uses `packageManager: 'pnpm'`
- [ ] All moon.yml task files updated (pnpm run, pnpm exec)
- [ ] package-lock.json files removed
- [ ] pnpm-lock.yaml generated in each package directory
- [ ] `pnpm install` completes successfully in all packages
- [ ] `moon run web/app:build` succeeds
- [ ] `moon run web/app:dev` works in development
- [ ] `moon run server/cortex:test` (Playwright) works
- [ ] Native dependencies (Playwright, etc.) work on both macOS and Linux

### 8. Documentation Updates

Files to update:
- `/src/README.md` - Replace npm commands with pnpm
- `/src/web/app/AGENTS.md` - Update development instructions
- Any other docs referencing npm commands

### 9. Common Commands Mapping

| npm Command | pnpm Equivalent |
|-------------|----------------|
| `npm install` | `pnpm install` |
| `npm install <pkg>` | `pnpm add <pkg>` |
| `npm uninstall <pkg>` | `pnpm remove <pkg>` |
| `npm run <script>` | `pnpm run <script>` or `pnpm <script>` |
| `npx <cmd>` | `pnpm exec <cmd>` or `pnpm dlx <cmd>` |
| `npm install -g <pkg>` | `pnpm add -g <pkg>` |
| `npm update` | `pnpm update` |
| `npm list` | `pnpm list` |

### 10. Rollback Plan

If migration encounters issues:

1. **Revert Git changes:**
   ```bash
   git checkout main
   git branch -D npm-to-pnpm-migration
   ```

2. **Reinstall with npm:**
   ```bash
   find . -name "node_modules" -type d -prune -exec rm -rf {} +
   find . -name "pnpm-lock.yaml" -delete
   npm install
   ```

3. **Document issues** for future migration attempt

## Cross-Platform Artifact Configuration Details

### Why This Matters

Some npm packages include native binaries (e.g., Playwright browsers, esbuild, sharp, etc.) that are platform-specific. By default, package managers only download artifacts for the current platform.

**Problem**: Developers on macOS need linux binaries when building or testing for production Linux environments. CI running on Linux might need to test darwin builds.

**Solution**: Configure pnpm to download artifacts for multiple platforms:

```ini
# .npmrc
supportedArchitectures[os][]=darwin
supportedArchitectures[os][]=linux
supportedArchitectures[cpu][]=arm64
supportedArchitectures[cpu][]=x64
```

This ensures that when Playwright (or similar packages) is installed, pnpm downloads:
- darwin-arm64 binaries
- darwin-x64 binaries
- linux-arm64 binaries
- linux-x64 binaries

### Trade-offs

**Pros:**
- Eliminates "binary not found" errors when switching platforms
- Development on macOS with deployment to Linux just works
- Lock files work consistently across all platforms

**Cons:**
- Larger disk usage (4x binaries for packages with native deps)
- Slower initial install (downloads multiple platform variants)
- Most packages don't have native binaries, so overhead is minimal

### Selective Configuration

If disk space is a concern, configure only for current architecture:

```bash
# Detect current arch and configure
ARCH=$(uname -m)
if [ "$ARCH" = "arm64" ] || [ "$ARCH" = "aarch64" ]; then
  pnpm config set supportedArchitectures.cpu "arm64"
else
  pnpm config set supportedArchitectures.cpu "x64"
fi

# Always support both darwin and linux OS
pnpm config set supportedArchitectures.os "darwin,linux"
```

## Timeline Estimate

- **Preparation**: 1-2 hours (create configs, update moon configuration files)
- **Execution**: 30 minutes (remove npm artifacts, install with pnpm)
- **Testing**: 2-3 hours (verify all builds, tests, and deployments)
- **Documentation**: 1 hour (update README and docs)

**Total**: ~1 day for full migration and verification

## Success Criteria

1. All web packages build successfully with `moon run :build`
2. All tests pass with `moon run :test`
3. Development servers start with `moon run web/app:dev`
4. Playwright tests work on both macOS and Linux
5. CI/CD pipelines complete successfully
6. Developer documentation updated

## References

- [pnpm Documentation](https://pnpm.io/)
- [pnpm file: Protocol](https://pnpm.io/cli/install#file-protocol)
- [Moon Package Manager Config](https://moonrepo.dev/docs/config/toolchain#node)
- [pnpm .npmrc Config](https://pnpm.io/npmrc)
- [Supported Architectures](https://pnpm.io/package_json#supportedarchitectures)
