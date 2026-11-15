# API Reference

Base URL (local): `http://localhost:8080`

All REST endpoints are versioned under `/api/v1`. Request and response bodies are JSON
(`Content-Type: application/json`). Request bodies are limited to 1 MB.

## Contents

- [Response envelope](#response-envelope)
- [Authentication](#authentication)
- [Pagination](#pagination)
- [Errors](#errors)
- [Rate limiting](#rate-limiting)
- [CORS](#cors)
- [Endpoints](#endpoints)
  - [GET /health](#get-health)
  - [POST /api/v1/auth/register](#post-apiv1authregister)
  - [POST /api/v1/auth/login](#post-apiv1authlogin)
  - [GET /api/v1/users/me](#get-apiv1usersme)
  - [GET /api/v1/users](#get-apiv1users)
  - [GET /api/v1/users/{id}](#get-apiv1usersid)
  - [POST /api/v1/messages](#post-apiv1messages)
  - [GET /api/v1/messages/{userID}](#get-apiv1messagesuserid)
- [WebSocket](#websocket)
- [Data models](#data-models)

## Response envelope

Every JSON response uses the same envelope:

```json
{ "success": true, "data": { ... } }
```

```json
{ "success": false, "error": "human readable message" }
```

| Field     | Type    | Notes                                   |
|-----------|---------|-----------------------------------------|
| `success` | boolean | Always present                          |
| `data`    | any     | Present on success (omitted when empty) |
| `error`   | string  | Present on failure                      |

Exception: `GET /health` returns `data` (not `error`) even when `success` is `false`.

## Authentication

Login and register return a JWT access token (HS256). Its lifetime is set by `JWT_EXPIRY`
(default `24h`); `expires_in` in the response is that lifetime in seconds.

Send it on protected endpoints:

```
Authorization: Bearer <access_token>
```

For the WebSocket endpoint, browsers cannot set headers, so the token may be passed as a
query parameter instead: `/ws?token=<access_token>`. The `?token=` parameter is only accepted
on WebSocket upgrade requests (`Upgrade: websocket`); the `Authorization` header also works there.

Auth failures (status `401`):

| `error`                            | When                                  |
|------------------------------------|---------------------------------------|
| `missing access token`             | No Bearer token (and no `?token=` on WS upgrade) |
| `invalid or expired access token`  | Bad signature, wrong algorithm, expired, or no user id |

## Pagination

List endpoints accept query parameters:

| Param    | Default | Notes                                                 |
|----------|---------|-------------------------------------------------------|
| `limit`  | `20`    | Values `<= 0` or non-numeric use the default; capped at `100` |
| `offset` | `0`     | Negative or non-numeric values become `0`             |

## Errors

Common status codes and their `error` messages:

| Status | `error`                         | Cause                                         |
|--------|---------------------------------|-----------------------------------------------|
| 400    | `invalid JSON body`             | Body is not valid JSON, has wrong field types, or exceeds 1 MB |
| 400    | `invalid input: <detail>`       | Validation failed (details per endpoint)      |
| 400    | `invalid user id`               | Path id is not a positive integer             |
| 401    | `missing access token` / `invalid or expired access token` | See [Authentication](#authentication) |
| 401    | `invalid email or password`     | Login failed                                  |
| 404    | `not found`                     | Resource does not exist                       |
| 409    | `email or username already in use` | Register with an existing email/username   |
| 429    | `too many requests`             | See [Rate limiting](#rate-limiting)           |
| 500    | `internal server error`         | Unexpected server error (details are logged, never returned) |

Unknown paths (`404`) and wrong methods (`405`) are answered by Go's router with a plain-text
body, not the JSON envelope.

## Rate limiting

Every request (including `/health` and `/ws`) passes through an in-memory token bucket per
client IP: `RATE_LIMIT_RPS` requests/second sustained (default `10`) with bursts up to
`RATE_LIMIT_BURST` (default `20`). CORS preflight (`OPTIONS`) requests are answered before the
limiter and are not counted.

When the limit is exceeded:

```
HTTP/1.1 429 Too Many Requests
Retry-After: 1
Content-Type: application/json; charset=utf-8

{"success":false,"error":"too many requests"}
```

The client IP is the TCP peer address; `X-Forwarded-For` is not used.

## CORS

Allowed origins come from `CORS_ALLOWED_ORIGINS` (comma-separated, `*` = any). For allowed
origins the API returns `Access-Control-Allow-Origin: <origin>` (the request origin echoed back),
methods `GET, POST, PUT, PATCH, DELETE, OPTIONS`, headers `Authorization, Content-Type`, and
`Access-Control-Max-Age: 600`. `Access-Control-Allow-Credentials: true` is sent only when the
origin list is explicit; it is **not** sent when the list contains `*`. Preflight requests get
`204 No Content`. The same origin list is used to check the `Origin` header on WebSocket upgrades.

## Endpoints

| Method | Path                         | Auth |
|--------|------------------------------|------|
| GET    | `/health`                    | No   |
| POST   | `/api/v1/auth/register`      | No   |
| POST   | `/api/v1/auth/login`         | No   |
| GET    | `/api/v1/users/me`           | Yes  |
| GET    | `/api/v1/users`              | Yes  |
| GET    | `/api/v1/users/{id}`         | Yes  |
| POST   | `/api/v1/messages`           | Yes  |
| GET    | `/api/v1/messages/{userID}`  | Yes  |
| GET    | `/ws`                        | Yes (`?token=` or Bearer) |

---

### GET /health

Checks PostgreSQL and Redis (2 s timeout). No auth.

**200 OK** — all dependencies up:

```json
{
  "success": true,
  "data": {
    "status": "ok",
    "services": { "postgres": "up", "redis": "up" }
  }
}
```

**503 Service Unavailable** — at least one dependency down:

```json
{
  "success": false,
  "data": {
    "status": "degraded",
    "services": { "postgres": "up", "redis": "down" }
  }
}
```

---

### POST /api/v1/auth/register

Creates an account and returns a token. No auth.

**Request**

```json
{
  "username": "alice",
  "email": "alice@example.com",
  "password": "s3cretpass"
}
```

| Field      | Rules                                                                 |
|------------|-----------------------------------------------------------------------|
| `username` | Trimmed; 3-50 characters                                              |
| `email`    | Trimmed and lowercased; must be a bare address (`a@b.com`, not `Bob <a@b.com>`); max 255 characters |
| `password` | 8-72 bytes (bcrypt limit)                                             |

Fields are validated in that order; the first failure is returned.

**201 Created**

```json
{
  "success": true,
  "data": {
    "user": {
      "id": 1,
      "username": "alice",
      "email": "alice@example.com",
      "created_at": "2026-11-15T10:00:00.000000Z",
      "updated_at": "2026-11-15T10:00:00.000000Z"
    },
    "access_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
    "expires_in": 86400
  }
}
```

**Errors**

| Status | `error`                                              |
|--------|------------------------------------------------------|
| 400    | `invalid JSON body`                                  |
| 400    | `invalid input: username must be 3-50 characters`    |
| 400    | `invalid input: email is not valid` (unparseable, display-name form, or > 255 chars) |
| 400    | `invalid input: password must be 8-72 characters`    |
| 409    | `email or username already in use`                   |
| 500    | `internal server error`                              |

---

### POST /api/v1/auth/login

No auth.

**Request**

```json
{
  "email": "alice@example.com",
  "password": "s3cretpass"
}
```

`email` is trimmed and lowercased before lookup.

**200 OK** — same shape as register:

```json
{
  "success": true,
  "data": {
    "user": { "id": 1, "username": "alice", "email": "alice@example.com", "created_at": "...", "updated_at": "..." },
    "access_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
    "expires_in": 86400
  }
}
```

**Errors**

| Status | `error`                       |
|--------|-------------------------------|
| 400    | `invalid JSON body`           |
| 401    | `invalid email or password` (unknown email or wrong password) |
| 500    | `internal server error`       |

---

### GET /api/v1/users/me

Returns the authenticated user, including `email`. Auth required.

**200 OK**

```json
{
  "success": true,
  "data": {
    "id": 1,
    "username": "alice",
    "email": "alice@example.com",
    "created_at": "2026-11-15T10:00:00.000000Z",
    "updated_at": "2026-11-15T10:00:00.000000Z"
  }
}
```

**Errors**

| Status | `error`                         |
|--------|---------------------------------|
| 401    | `missing access token` / `invalid or expired access token` |
| 404    | `not found` (user in token no longer exists) |
| 500    | `internal server error`         |

---

### GET /api/v1/users

Lists users ordered by `id` ascending. Auth required.

**Query:** `limit`, `offset` (see [Pagination](#pagination)).

**200 OK** — `data` is an array (empty array `[]` when there are no results). Emails are
private: `email` is omitted for every user except the caller's own record. Example as user 1:

```json
{
  "success": true,
  "data": [
    { "id": 1, "username": "alice", "email": "alice@example.com", "created_at": "...", "updated_at": "..." },
    { "id": 2, "username": "bob", "created_at": "...", "updated_at": "..." }
  ]
}
```

**Errors**

| Status | `error`                         |
|--------|---------------------------------|
| 401    | `missing access token` / `invalid or expired access token` |
| 500    | `internal server error`         |

---

### GET /api/v1/users/{id}

Returns one user. Auth required.

**Path:** `id` — positive integer.

**200 OK** — a [User](#user) object in `data`. `email` is omitted unless `id` is the caller's own id:

```json
{
  "success": true,
  "data": { "id": 2, "username": "bob", "created_at": "...", "updated_at": "..." }
}
```

**Errors**

| Status | `error`                         |
|--------|---------------------------------|
| 400    | `invalid user id`               |
| 401    | `missing access token` / `invalid or expired access token` |
| 404    | `not found`                     |
| 500    | `internal server error`         |

---

### POST /api/v1/messages

Sends a direct message from the authenticated user. Auth required.

On success the message is also pushed over WebSocket as a `message:new` event to every open
connection of the receiver **and** of the sender (to sync other tabs/devices).

**Request**

```json
{
  "receiver_id": 2,
  "content": "Hello Bob!"
}
```

| Field         | Rules                                                     |
|---------------|-----------------------------------------------------------|
| `content`     | Trimmed; 1-2000 characters                                |
| `receiver_id` | Positive integer; must not be the sender's own id; user must exist |

`content` is validated before `receiver_id`.

**201 Created**

```json
{
  "success": true,
  "data": {
    "id": 10,
    "sender_id": 1,
    "receiver_id": 2,
    "content": "Hello Bob!",
    "created_at": "2026-11-15T10:05:00.000000Z"
  }
}
```

**Errors**

| Status | `error`                                              |
|--------|------------------------------------------------------|
| 400    | `invalid JSON body`                                  |
| 400    | `invalid input: content must be 1-2000 characters`   |
| 400    | `invalid input: receiver_id is not valid`            |
| 401    | `missing access token` / `invalid or expired access token` |
| 404    | `not found: receiver does not exist`                 |
| 500    | `internal server error`                              |

---

### GET /api/v1/messages/{userID}

Returns the conversation between the authenticated user and `userID`, **newest first**
(`created_at DESC, id DESC`). Auth required.

**Path:** `userID` — positive integer (the other participant).
**Query:** `limit`, `offset` (see [Pagination](#pagination)).

**200 OK** — array of [Message](#message) objects (`[]` if none; also `[]` if `userID` does not exist):

```json
{
  "success": true,
  "data": [
    { "id": 11, "sender_id": 2, "receiver_id": 1, "content": "Hi Alice", "created_at": "2026-11-15T10:06:00.000000Z" },
    { "id": 10, "sender_id": 1, "receiver_id": 2, "content": "Hello Bob!", "created_at": "2026-11-15T10:05:00.000000Z" }
  ]
}
```

**Errors**

| Status | `error`                         |
|--------|---------------------------------|
| 400    | `invalid user id`               |
| 401    | `missing access token` / `invalid or expired access token` |
| 500    | `internal server error`         |

---

## WebSocket

```
GET /ws?token=<access_token>
```

Example: `ws://localhost:8080/ws?token=eyJhbGciOi...`

- Authentication happens before the upgrade. On failure the server responds with HTTP `401`
  and the JSON envelope (`missing access token` / `invalid or expired access token`).
- The `Origin` header must be empty or listed in `CORS_ALLOWED_ORIGINS` (or that list contains
  `*`); otherwise the upgrade fails with `403`.
- A user may hold several connections at once (tabs/devices); events go to all of them.
- The connection is **server-to-client only**: frames sent by the client are read and ignored
  (max 4096 bytes per frame; larger frames close the connection). Send messages via
  `POST /api/v1/messages`.
- The server pings every 54 s; the client must answer (browsers do this automatically) or the
  connection is closed after 60 s without a pong.
- If a client's 32-event send buffer fills up (slow consumer) the server drops that connection;
  clients should reconnect.

### Event format

Each event is one JSON text frame:

```json
{ "type": "<event type>", "data": { ... } }
```

### `message:new`

Sent when a message is created (to both receiver and sender). `data` is a [Message](#message):

```json
{
  "type": "message:new",
  "data": {
    "id": 10,
    "sender_id": 1,
    "receiver_id": 2,
    "content": "Hello Bob!",
    "created_at": "2026-11-15T10:05:00.000000Z"
  }
}
```

### Browser example

```js
const ws = new WebSocket(`ws://localhost:8080/ws?token=${accessToken}`);
ws.onmessage = (e) => {
  const evt = JSON.parse(e.data);
  if (evt.type === "message:new") console.log("new message", evt.data);
};
```

## Data models

Timestamps are RFC 3339 strings as produced by Go's `time.Time` JSON encoding
(e.g. `2026-11-15T10:05:00.123456Z`).

### User

| Field        | Type    |
|--------------|---------|
| `id`         | integer |
| `username`   | string  |
| `email`      | string (optional) |
| `created_at` | string (timestamp) |
| `updated_at` | string (timestamp) |

`email` is `omitempty`: it is present only in the caller's own record (`/users/me`, register/login
responses, and the caller's own entry in `/users` or `/users/{id}`); it is omitted for other users.
The password hash is never serialized.

### Message

| Field         | Type    |
|---------------|---------|
| `id`          | integer |
| `sender_id`   | integer |
| `receiver_id` | integer |
| `content`     | string  |
| `created_at`  | string (timestamp) |

### AuthResult

| Field          | Type    | Notes                         |
|----------------|---------|-------------------------------|
| `user`         | [User](#user) |                         |
| `access_token` | string  | JWT (HS256), claims `uid`, `sub`, `iat`, `exp` |
| `expires_in`   | integer | Token lifetime in seconds     |
