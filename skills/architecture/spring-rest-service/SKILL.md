---
name: spring-rest-service
description: Base architecture for Spring Boot REST Microservices.
---

# Architecture Profile: Spring Boot REST Service

## Structure
- `src/main/java/com/example/domain`: Entities and Value Objects
- `src/main/java/com/example/application`: Use Cases and Services
- `src/main/java/com/example/infrastructure`: Adapters, Repositories, Controllers

## Rules
- controllers MUST ONLY delegate to application services.
- domain layer MUST NOT have external dependencies.
