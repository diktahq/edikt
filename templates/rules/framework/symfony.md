---
paths: "**/*.php"
version: "0.1.0"
---
<!-- edikt:generated -->

# Symfony

Rules for building Symfony applications.

## Critical

- NEVER call `$container->get()` inside a service or controller — that is service location, not dependency injection. It defeats the container's ability to manage dependencies and makes testing impossible without a full container.
- NEVER check roles directly in business logic. Authorization belongs in voters (`#[IsGranted]` or `$this->denyAccessUnlessGranted()`), not in if/else conditionals inside services.
- MUST use constructor injection exclusively. Services are autowired by default — let the container wire them.

## Standards

- Type-hint interfaces in constructors when multiple implementations exist. When there is only one implementation, concrete type-hinting is acceptable — don't introduce an interface for one class.
- Define entities with PHP attributes, not XML or YAML mappings. Attributes keep mapping co-located with the entity definition.
- Use custom repositories that extend `ServiceEntityRepository`. All query logic lives in repositories — never in controllers or services.
- Use Doctrine QueryBuilder or DQL for complex queries. Define indexes on columns used in WHERE and ORDER BY clauses.
- Use `php bin/console make:migration` for ALL schema changes. Never alter the database manually or modify existing migrations that have been deployed.
- Use Symfony Messenger for async operations (emails, notifications, external API calls). Messages are simple DTOs. Handlers contain the logic. Use `#[AsMessageHandler]`.

## Practices

- Return typed responses: `JsonResponse`, `Response`, `RedirectResponse`. Don't return raw strings.
- Use `#[MapRequestPayload]` or Form types for request deserialization and validation. Don't manually read from `$request->getContent()` and decode.
- Use `WebTestCase` for controller tests (HTTP layer). Use `KernelTestCase` for service integration tests. Reset the database between tests with `dama/doctrine-test-bundle`.

## Critical

- NEVER use service location (`$container->get()`) inside services.
- NEVER check authorization in business logic — use voters.
