# Contract checks

The `contract-check` Compose profile publishes the OpenAPI and Protobuf
contracts and serves fixed HTTP and gRPC examples. It does not implement
business rules, persistence, or service readiness.

The future services will implement internal `/healthz` and `/readyz` endpoints.
They will add `/metrics` with observability. Readiness requires successful
migrations and access to the service's own database. The contract-check servers
must not be treated as ready production services.
