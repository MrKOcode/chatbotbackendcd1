Project Name

AIChatBot

Project Overview

AIChatBot is an educational chatbot platform for students and administrators.

Students can:

- create conversations
- ask questions
- review conversational history
- complete assessments



Administrators can:

- manage users
- configure AI services
- review students' conversational history

Current System Status

The project has a working frontend and backend integration.

Current working functionality:

- User registration
- User login
- Authentication
- AI chat
- Frontend and backend communication
- AWS deployment

Known issues:

- Conversation History is currently not functioning correctly.
- Student authorization for conversation history needs improvement.

The current goal is to improve these issues while preserving existing working functionality.

Current Focus:

The current development priority is fixing and improving the conversation history feature.

Priority 1
- Fix the chat history feature, which is currently not functioning.

Priority 2
- Ensure authenticated students can only view their own conversation history.
- Verify that authorization is enforced on both the backend and API responses.

Priority 3
- Stabilize the backend after the chat history feature is working.

Priority 4
- Improve automated testing.

Priority 5
- Improve CI/CD and deployment readiness.

Priority 6
- Continue improving AWS deployment and production readiness.



Backend
- Go
- Gin
- AWS Lambda
- DynamoDB
- Cognito

Frontend

- React
- Redux
- Typescript

Deployment

- AWS
- GitHub Actions

Coding Style

- Clean Architecture
- REST APIs
- Unit tests required

Current Architecture

The backend is organized into multiple Go components behind an API Gateway.

Major components include:

- Authentication
- AI Chat
- Conversation History
- Administration

The frontend communicates only through the API Gateway.

