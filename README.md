# AI Chatbot 3.0 Backend

Serverless backend for an academic AI chatbot. The application runs on AWS and provides Cognito-based authentication, role-aware conversation access, DynamoDB persistence, and OpenAI-generated replies.

## Architecture

```text
Web client
   |
   v
Amazon API Gateway
   |-- /api/auth/* --------> Auth Lambda ------> Cognito JWKS
   |
   `-- /api/AIchat/* ------> AIChat Lambda
                                  |-- DynamoDB (conversations and messages)
                                  `-- OpenAI Chat Completions API

Amazon Cognito
   |-- Hosted UI and user pool
   |-- students group
   `-- admins group
```

The infrastructure is declared in `backend/template.yaml` with AWS SAM. Deploying the stack creates:

- an API Gateway REST API with CORS and a Cognito authorizer;
- a Cognito user pool, hosted UI domain, app client, and `students`/`admins` groups;
- an auto-confirm pre-sign-up Lambda;
- the Go `Auth` and `AIChat` Lambda functions; and
- a pay-per-request DynamoDB table with a `GSI1` index for user timeline queries.

## Repository structure

```text
.
|-- .github/workflows/backend-ci.yml    # Formatting, tests, coverage, and SAM build
|-- backend/
|   |-- components/
|   |   |-- AIChat/
|   |   |   |-- main.go                # Chat API routing and authorization
|   |   |   |-- main_test.go
|   |   |   `-- services/
|   |   |       |-- dal.go             # Storage interface and domain models
|   |   |       |-- dynamo_dal.go      # DynamoDB implementation
|   |   |       `-- chatgpt.go         # OpenAI client
|   |   `-- Auth/
|   |       |-- main.go                # Cognito JWT validation and role response
|   |       `-- main_test.go
|   |-- template.yaml                  # AWS SAM infrastructure
|   |-- samconfig.toml                 # Default and isolated test deploy profiles
|   `-- build_all.sh                   # Builds Linux Lambda executables
|-- Design/                            # Architecture diagrams
`-- AiChatBotBackend.sln               # Visual Studio solution
```

The root `package.json` contains frontend-oriented packages and is not required to build the Go backend.

## API overview

Except for `/api/auth/*` and CORS preflight requests, API Gateway requires a valid Cognito access token.

| Method | Path | Purpose |
| --- | --- | --- |
| `ANY` | `/api/auth/{proxy+}` | Validate a bearer token and return `userId`, `email`, `role`, and groups |
| `POST` | `/api/AIchat/conversations` | Create a conversation and its initial greeting |
| `GET` | `/api/AIchat/conversations` | List the current user's conversations |
| `GET` | `/api/AIchat/conversations/{id}/messages` | List messages in a conversation |
| `POST` | `/api/AIchat/conversations/{id}/messages` | Store a user message and generate/store the assistant reply |
| `DELETE` | `/api/AIchat/conversations/{id}` | Delete a conversation and its messages |
| `GET` | `/api/AIchat/history` | Return the current user's message history |

Admins can add `?targetUserId=<cognito-sub>` to supported chat requests to act on another user's data. Students can access only their own data.

The send-message body has this shape:

```json
{
  "message": {
    "conversationId": "01J...",
    "content": "Explain recursion with an example"
  }
}
```

Send the Cognito token on protected requests:

```http
Authorization: Bearer <token>
Content-Type: application/json
```

## Prerequisites

- Go 1.24.3 or newer
- AWS CLI configured with credentials for the target account
- AWS SAM CLI
- an OpenAI API key
- Bash, WSL, or another Unix-like shell for `build_all.sh`

## Configuration

The SAM template accepts the following parameters. Override them at deploy time to keep environments isolated.

| Parameter | Default | Description |
| --- | --- | --- |
| `AIChatFunctionName` | `AIChatHandler` | Chat Lambda name |
| `AuthFunctionName` | `AuthHandler` | Token-validation Lambda name |
| `AutoConfirmFunctionName` | `AutoConfirmHandler` | Cognito trigger Lambda name |
| `ChatTableName` | `ChatMessagesTableV4` | DynamoDB table name |
| `UserPoolName` | `mychatbot-user-pool` | Cognito user pool name |
| `UserPoolClientName` | `mychatbot-web` | Cognito app client name |
| `CognitoDomainPrefix` | `mychatbot-3-0` | Hosted UI domain prefix |
| `ApiStageName` | `Prod` | API Gateway stage |
| `AllowedCorsOrigin` | deployed frontend URL | Browser origin allowed by API Gateway |
| `OpenAISecretId` | `mychatbot/openai` | Secrets Manager secret containing the API key |

Create the OpenAI secret before deploying. The secret value must be a JSON object with an `OPENAI_API_KEY` field:

```powershell
aws secretsmanager create-secret `
  --name mychatbot/openai `
  --secret-string '{"OPENAI_API_KEY":"replace-me"}' `
  --region us-west-2
```

Do not commit API keys or `.env` files. At runtime, SAM injects `TABLE_NAME`, Cognito values, `API_STAGE_NAME`, `ALLOWED_ORIGIN`, and the resolved `OPENAI_API_KEY` into the Lambda functions.

## Build and test

Run tests independently for both Go modules:

```powershell
Set-Location backend/components/AIChat
go test ./...

Set-Location ../Auth
go test ./...
```

Build the Lambda executables and validate the SAM template:

```bash
cd backend
./build_all.sh
sam validate --lint
sam build
```

CI performs `gofmt` checks, runs both test suites with a minimum 75% coverage threshold, builds the Lambda executables, and runs `sam build`.

## Deploy

The default SAM profile deploys the `MyChatbot3backend` stack in `us-west-2` and asks for change-set confirmation:

```powershell
Set-Location backend
sam build
sam deploy
```

For an isolated test stack, use the included `codex-test` profile:

```powershell
Set-Location backend
sam build
sam deploy --config-env codex-test
```

After deployment, SAM prints the API endpoint, Cognito user-pool and client IDs, hosted UI URL, and DynamoDB table name. Update the Cognito callback/logout URLs and `AllowedCorsOrigin` for the frontend environment you actually deploy.

## Data model

DynamoDB uses a single-table design:

- conversation headers are stored under `PK=USER#<userId>` and `SK=CONV#<conversationId>`;
- messages are stored under `PK=CONV#<conversationId>` and time-ordered `MSG#...` sort keys; and
- `GSI1` indexes messages by user and timestamp for history queries.

Pagination tokens are base64-encoded DynamoDB `LastEvaluatedKey` values.

## Operational notes

- The deployed AI client currently calls the OpenAI Chat Completions endpoint with the `gpt-4` model identifier.
- The Cognito pre-sign-up trigger automatically confirms users and verifies their email. Review this behavior before using the stack in a production environment.
- `backend/samconfig.toml` contains environment-specific names and a fixed AWS region; adjust these for additional environments.
- Legacy local-service helper scripts remain under `backend/`, but the supported build and runtime path described here is AWS SAM.

## License

No project license is currently declared.
