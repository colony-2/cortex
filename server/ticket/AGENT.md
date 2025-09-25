# VibeThis Ticket Agent Overview

The `server/ticket` project encapsulates the ticketing domain for VibeThis. It exposes a Go package (`pkg/ticket`) that other services can depend on to create, update, and search tickets tied to dependency graph cells. Tickets carry stage/state metadata, capture the current actor (user or automation), and enforce optimistic concurrency so that workflows can coordinate safely.

Key capabilities provided here:

- **Data model**: Types for stages, states, actors, and `Ticket` records, plus the event log schema (`TicketEvent`, `TicketReset`) with discriminator-backed payloads and per-kind embedded structures.
- **Storage layer**: GORM-backed repositories for both tickets and ticket events (`internal/store/tickets`, `internal/store/events`) that handle migrations, transactional updates/resets, and iterator-based search queries.
- **Service layer**: Business logic (`internal/service`) for ticket lifecycle operations and append/list/reset semantics on the event log, including validator-backed payload enforcement, automatic base58 ID generation, and helper constructors for common actors.
- **Public package**: `pkg/ticket` re-exports the domain types, error constants, service interfaces, and event utilities for downstream consumers.
- **Test harness**: Shared embedded Postgres utilities (`internal/testutil`) powering integration coverage for both ticket CRUD and event log scenarios.
Consumers should import `github.com/divisive-ai/vibethis/server/ticket/pkg/ticket` to interact with the service and event APIs, relying on the provided `Service` interface, iterators, and helper constructors.
