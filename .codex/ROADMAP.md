# AIChatBot Roadmap

## Current Milestone

Fix and stabilize the Conversation History feature.

This is the highest priority because the backend and frontend are generally working, but chat history is currently not functioning correctly.

## Priority 1: Conversation History Fix

Goals:

* Make conversation history return correctly from the backend.
* Ensure students can only view their own conversation history.
* Ensure admins can view student histories only through authorized admin routes.
* Verify frontend and backend history API compatibility.
* Add tests for successful history retrieval, unauthorized access, and invalid requests.

Definition of done:

* Student login works.
* Chat still works.
* Student chat history works.
* Students cannot access another student's history.
* Admin history review works or is clearly documented as future work.
* Relevant tests pass.

## Priority 2: Backend Stabilization

Goals:

* Improve error handling.
* Make API responses more consistent.
* Remove duplicated logic.
* Clean up confusing code paths.
* Document environment variables.

Definition of done:

* Backend builds successfully.
* Existing working features are not broken.
* Known behavior is documented.

## Priority 3: Testing

Goals:

* Add unit tests for services.
* Add handler tests for API behavior.
* Add tests for authentication and authorization.
* Add regression tests for the chat history bug.

Definition of done:

* `go test ./...` passes from the correct Go module directory.
* Important success and failure cases are covered.

## Priority 4: CI/CD

Goals:

* Ensure GitHub Actions runs from the correct backend Go module location.
* Run formatting, build, and tests automatically.
* Prevent broken code from being pushed unnoticed.

Definition of done:

* GitHub Actions passes on push and pull request.
* Workflow matches the actual repository structure.

## Priority 5: AWS Deployment Readiness

Goals:

* Keep SAM template organized.
* Confirm Lambda/API Gateway deployment works.
* Document deployment steps.
* Avoid committing secrets or build artifacts.

Definition of done:

* Backend can be deployed using documented commands.
* Required AWS resources are documented.
* Deployment configuration is reproducible.

## Future Features

Potential future improvements:

* Self-assessment workflow
* Teacher dashboard
* Admin analytics
* Streaming AI responses
* File/material upload
* Better logging and monitoring
* Rate limiting
* CloudFront frontend deployment
* Production database design improvements

## Roadmap Rule

Do not start large new features until Conversation History, authorization, tests, and CI are stable.
