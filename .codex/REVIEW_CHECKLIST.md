# AIChatBot Review Checklist

## Before Accepting Code Changes

Check that the change:

* Fixes the requested issue only
* Does not rewrite unrelated working code
* Preserves frontend/backend compatibility
* Does not introduce new secrets or hardcoded credentials
* Uses clear Go naming and structure
* Has been formatted with `gofmt`

## Backend Checks

Verify that:

* The backend builds successfully
* Relevant tests pass
* API routes still match frontend expectations
* Request validation is handled
* Errors return proper HTTP status codes
* Internal errors are not exposed directly to clients

## Authentication and Authorization Checks

Verify that:

* Authenticated routes require valid authentication
* User identity comes from JWT/auth context
* Student users can only access their own data
* Admin-only behavior is protected by role checks
* Client-provided roles or user IDs are not blindly trusted

## Conversation History Checks

For chat history changes, verify that:

* Students can retrieve their own conversation history
* Students cannot retrieve another student's history
* Admin access is explicitly authorized
* Empty history returns a safe response
* Missing or invalid conversation IDs are handled
* History responses match what the frontend expects

## Testing Checks

Confirm that tests cover:

* Success case
* Invalid input
* Unauthorized request
* Forbidden access
* Not found case
* Database/service error where practical

## Documentation Checks

Update documentation when behavior changes:

* README.md
* .codex/PROJECT.md
* .codex/ARCHITECTURE.md
* .codex/API_GUIDE.md
* .codex/ROADMAP.md

## Git Checks

Before committing:


git status
go test ./...


Commit should be small and focused.

Good commit examples:

* `fix: repair conversation history retrieval`
* `test: add conversation history authorization tests`
* `docs: update API guide for history endpoints`
* `ci: run tests from backend module`
