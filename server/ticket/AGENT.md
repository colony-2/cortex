# VibeThis Ticket Agent Overview

The `server/ticket` project encapsulates the ticketing domain for VibeThis. It exposes a Go package (`pkg/ticket`) that other services can depend on to create, update, and search tickets tied to dependency graph cells. Tickets carry stage/state metadata, capture the current actor (user or automation), and enforce optimistic concurrency so that workflows can coordinate safely.

Key capabilities provided here:

- **Data model**: Types for stages, states, actors, and the `Ticket` record with optimistic lock versions and completion timestamps.
- **Storage layer**: GORM-backed repository (`internal/store/tickets`) that handles migrations, transactional updates, and iterator-based search queries.
- **Service layer**: Business logic (`internal/service`) for ticket lifecycle operations, validation of inputs, automatic base58 ID generation, and helper constructors for common actors.
- **Test harness**: Shared embedded Postgres utilities (`internal/testutil`) that drive integration tests without external dependencies.

Consumers should import `github.com/divisive-ai/vibethis/server/ticket/pkg/ticket` to interact with the service, relying on the provided `Service` interface and helpers.
