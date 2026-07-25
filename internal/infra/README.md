# infra

`infra` (infrastructure) directory — holds adapters in hexagonal architecture, and infrastructure implementations (repositories, message clients, external service adapters) in clean/onion architecture.

## What lives here
Outbound adapters / infrastructure implementations: 
db (postgresql/etc) repositories, eventbus and queue producers/consumers, external
service clients.

## Rules
- Port/interfaces are defined in the domain/app layer, NOT here. Adapters
  implement them; never define the contract here.
- No domain or business logic in this layer. Mapping, I/O, and error
  translation only.
- Dependencies point inward: `infra` may import `domain`/`app`; they must
  never import `infra`.
- Translate infra errors to domain errors at the boundary; don't leak driver
  types (`pgx`, `sarama`) upward.
