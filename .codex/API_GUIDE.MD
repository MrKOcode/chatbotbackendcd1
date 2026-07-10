# API Design Guide

## REST Principles

Use nouns instead of verbs.

Good:

GET /users
POST /users
GET /users/{id}

Avoid:

/getUsers
/createUser

---

## JSON Format

Success:

{
    "success": true,
    "data": {}
}

Failure:

{
    "success": false,
    "error": "description"
}

---

## HTTP Status Codes

200 OK

201 Created

400 Bad Request

401 Unauthorized

403 Forbidden

404 Not Found

409 Conflict

500 Internal Server Error

---

## Validation

Always validate:

- required fields
- data types
- string length
- IDs
- permissions



---

## Authentication

Authenticated routes must require valid JWTs.

Authorization must verify user permissions.

Never trust client-provided roles.

---

## Documentation

Every new endpoint should document:

- Purpose
- Request
- Response
- Errors
- Authentication

Current stable APIs

- Authentication
- Chat

Under development

- Conversation History

Future

- Self Assessment
- Admin Dashboard
