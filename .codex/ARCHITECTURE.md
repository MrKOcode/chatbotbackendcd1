# AIChatBot Architecture

## Overview

AIChatBot is an educational chatbot platform with separate student and administrator roles.

The system contains:

* React frontend
* API Gateway
* Authentication service
* AI Chat service
* Conversation History service
* Administration features
* Database storage

The frontend should communicate with the backend only through the API Gateway.

## High-Level Request Flow

User
  ↓
React Frontend
  ↓
API Gateway
  ↓
Backend Services
  ↓
Database / AI Provider


## Main Components

### 1. Frontend

The frontend is the user interface of AIChatBot.

Users can:

* register an account
* log in
* chat with the AI
* view conversation history
* log out

After logging in, the UI currently has three main navigation buttons:

* Chat
* Chat History
* Log Out

### 2. API Gateway

API Gateway is the main entry point for backend requests.

Responsibilities:

* receive requests from the frontend
* route requests to the correct backend service
* support CORS
* protect authenticated routes when required
* forward user identity information to backend services

API Gateway should not contain business logic.

### 3. Authentication

Authentication handles user login, registration, and identity verification.

There are two main account roles:

* Student
* Admin

Students can use the chatbot and view only their own conversation history.

Admins can manage users and review conversation histories across accounts.

Authentication may use Cognito/JWT depending on the current deployment configuration.

### 4. AI Chat

The AI Chat feature allows students to send messages and receive AI-generated responses.

Responsibilities:

* receive user messages
* validate request input
* identify the authenticated user
* call the AI service/provider
* save conversation messages
* return the AI response to the frontend

### 5. Conversation History

Conversation History allows users to review previous conversations.

Student behavior:

* students can only see their own conversation histories
* students must not be able to access another user's conversation history

Admin behavior:

* admins can view a list of users
* admins can review conversation histories for all users

This feature is currently the highest development priority because it is not working correctly.

### 6. Administration

Administration features are intended for admin users only.

Admins may be able to:

* view users
* manage user accounts
* review student conversation history
* configure AI-related settings

Admin routes must enforce authorization checks.

## Important Security Rule

Conversation history must always be filtered by authenticated user identity.

A student request should never be trusted to provide another user's ID directly.

Correct behavior:

Student logs in
  ↓
Backend reads student identity from JWT/auth context
  ↓
Backend queries only conversations owned by that student
  ↓
Backend returns only that student's history


Incorrect behavior:


Student sends userId in request
  ↓
Backend trusts that userId
  ↓
Student may access another user's history


## Current Development Priority

The current highest priority is fixing the Conversation History feature.

Goals:

1. Make chat history work correctly.
2. Ensure students can only view their own history.
3. Ensure admins can view history across users only through authorized admin routes.
4. Add tests for history success cases, unauthorized access, and invalid requests.
5. Keep frontend/backend API behavior documented.

## Intended Backend Boundaries

The backend should follow these boundaries:

* API Gateway handles routing.
* Authentication handles login, registration, and identity verification.
* Chat service handles AI conversation requests.
* Conversation History handles retrieving previous conversations.
* Database/repository layer handles storage access.
* Business logic should not be placed directly inside routing code when avoidable.

## Future Architecture Goals

Future improvements may include:

* stronger automated tests
* clearer API documentation
* better CI/CD
* improved AWS deployment configuration
* logging and monitoring
* rate limiting
* streaming AI responses
* teacher dashboard
* self-assessment workflow
