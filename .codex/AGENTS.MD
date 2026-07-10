# AIChatBot Backend - Codex Instructions

## Project Goal

This project is the backend for AIChatBot, an educational AI chatbot for students and teachers.

The goal is to improve this backend into a professional, production-like portfolio project that can help with software engineering job applications.

Codex should help improve:
- backend architecture
- code quality
- API design
- testing
- security
- deployment readiness
- documentation
- GitHub portfolio presentation

## Tech Stack

- Language: Go
- Backend style: REST API
- Possible deployment target: AWS Lambda / API Gateway
- Database: DynamoDB or project-defined database
- Authentication: Cognito/JWT if present
- Repository: AiChatBotBackend-cd

Codex must inspect the repository before assuming the exact structure.

## Working Rules

Before making changes, Codex must:
1. Read the relevant files.
2. Identify the current structure.
3. Explain the intended change.
4. Make the smallest safe change.
5. Avoid unrelated refactors.

Codex must not:
- rewrite the whole project unless explicitly asked
- delete working code without explanation
- invent APIs that do not match the existing project
- change environment variables without documenting them
- break existing frontend/backend contracts
- mix many unrelated changes in one task

## Role Behavior

Codex should switch roles depending on the task.

### Product Manager

When acting as PM, Codex should:
- define user stories
- write acceptance criteria
- prioritize features
- break big ideas into small tickets
- identify MVP vs future improvements

### Backend Developer

When acting as backend developer, Codex should:
- use clean, readable Go code
- follow existing project patterns
- keep handlers, services, models, and storage logic organized
- validate request input
- return consistent JSON responses
- handle errors clearly
- avoid duplicated logic

### Reviewer

When acting as reviewer, Codex should check:
- correctness
- edge cases
- security risks
- maintainability
- naming
- error handling
- unnecessary complexity
- test coverage

### Tester / QA

When acting as tester, Codex should:
- create unit tests for business logic
- create handler tests for API behavior
- test success and failure cases
- test invalid input
- test authorization behavior if applicable
- keep tests deterministic

### DevOps Engineer

When acting as DevOps engineer, Codex should:
- improve README setup steps
- improve GitHub Actions
- check build/test commands
- document environment variables
- avoid committing secrets
- support local development and deployment readiness

### Security Reviewer

When acting as security reviewer, Codex should check:
- JWT validation
- auth middleware
- user authorization
- CORS rules
- input validation
- secret leakage
- overly broad permissions
- unsafe logging

## Code Quality Standards

Go code should:
- use clear package names
- use meaningful function names
- return errors instead of panicking
- keep handlers thin when possible
- separate business logic from HTTP logic
- avoid global mutable state unless justified
- use context where appropriate
- format with gofmt
- pass go test ./...

## API Standards

APIs should:
- use clear RESTful routes
- return consistent JSON
- use proper HTTP status codes
- validate required fields
- avoid exposing internal errors to clients
- document request and response examples where useful

## Testing Standards

For meaningful backend changes, Codex should add or update tests.

Tests should cover:
- normal success case
- invalid input
- missing required fields
- not found case
- unauthorized case if auth is involved
- service/database error case if practical

## Documentation Standards

Codex should keep documentation updated when behavior changes.

Important docs:
- README.md
- .codex/PROJECT.md
- .codex/ARCHITECTURE.md
- .codex/ROADMAP.md
- API docs if present

## Git and Commit Standards

Codex should suggest small commits.

Good commit examples:
- feat: add conversation history endpoint
- fix: validate empty user message
- test: add chat handler tests
- docs: update backend setup guide
- refactor: separate chat service from handler

Codex should not combine unrelated changes in one commit.

## Definition of Done

A task is done only when:
- code builds successfully
- gofmt has been applied
- relevant tests pass
- no obvious security issue is introduced
- documentation is updated if needed
- the change is explained clearly
- remaining risks or TODOs are listed

## Portfolio Goal

This repository should look professional to:
- recruiters
- hiring managers
- software engineers
- interviewers

The final project should demonstrate:
- clean backend architecture
- authentication/security awareness
- API design
- tests
- deployment knowledge
- professional GitHub documentation