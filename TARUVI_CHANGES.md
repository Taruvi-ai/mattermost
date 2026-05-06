# Taruvi-Specific Code Changes

This document describes all custom code added on top of the Mattermost 11.3.0 fork for the Taruvi integration. The changes replace Mattermost's native authentication with Taruvi's auth system (and optionally Keycloak), and add the infrastructure to build and deploy the custom image.

---

## Overview

The core change is that **Mattermost's built-in password authentication is completely bypassed**. Instead, every login is delegated to an external Taruvi backend (or Keycloak). The Mattermost user record is still looked up in the local database after external auth succeeds — users must be pre-created in Mattermost.

Three authentication modes are supported:

| Mode | Trigger | Flow |
|---|---|---|
| Session token | `auth_type: "session"` in login request | Validates an allauth session token against Taruvi's session endpoint |
| JWT token | Password field starts with `eyJ` | Verifies JWT with Taruvi's verify endpoint, then fetches user info |
| Password | Default (Keycloak disabled) | Posts credentials to Taruvi's allauth login endpoint |
| Keycloak | `MM_KEYCLOAKSETTINGS_ENABLE=true` | Posts credentials to Keycloak's token endpoint |

---

## New Files

### 1. `server/public/model/taruvi.go`

**What it is:** Data model definitions for the Taruvi auth integration.

**Contents:**
- `UserAuthServiceTaruvi = "taruvi"` — constant identifying the auth service.
- `TaruviAuthRequest` — request body sent to Taruvi's login endpoint (email or username + password).
- `TaruviAuthResponse` — response from Taruvi's login/session endpoints, containing user info (`id`, `display`, `email`, `username`) and auth metadata (`session_token`, `access_token`, `refresh_token`, `expires_in`, `is_authenticated`).
- `TaruviUserResponse` — response from Taruvi's `/api/users/me/` endpoint (used when authenticating via JWT).

---

### 2. `server/channels/app/taruvi.go`

**What it is:** The Taruvi authentication provider — the main implementation of all Taruvi auth logic.

**Key components:**

- **`TaruviInterface`** (line 19) — interface with a single method `AuthenticateUser(rctx, username, password, authType)`.
- **`TaruviProvider`** (line 23) — struct holding a reference to the `App`.
- **`App.Taruvi()`** (line 27) — factory method on `App` that returns a `TaruviProvider`.

**`AuthenticateUser`** (line 31) — entry point, dispatches to one of three sub-methods:
  - `authType == "session"` → `validateSessionToken`
  - password starts with `eyJ` → `verifyJWTToken`
  - otherwise → `authenticateWithPassword`

**`authenticateWithPassword`** (line 127) — POSTs `{email, password}` or `{username, password}` to `TaruviServerURL + AuthEndpoint`. Reads `TaruviAuthResponse` from the response.

**`verifyJWTToken`** (line 68) — POSTs `{token}` to `TaruviServerURL + VerifyEndpoint`. On success, calls `getUserInfoWithToken`.

**`getUserInfoWithToken`** (line 218) — GETs `TaruviServerURL + UserEndpoint` with `Authorization: Bearer <token>`. Parses the `{data: {...}}` wrapper and maps `TaruviUserResponse` fields into a `TaruviAuthResponse`.

**`validateSessionToken`** (line 271) — GETs `TaruviServerURL + SessionEndpoint` with `X-Session-Token: <token>` header. Returns a `TaruviAuthResponse`.

**Host override** — all four HTTP methods respect `TaruviSettings.OverrideHost` / `HostOverrideValue`, which sets the HTTP `Host` header on outgoing requests. This is needed when the Taruvi server is accessed via a Docker network name but expects a different `Host` header.

**Helper functions:**
- `getPrefix(s, n)` — safely returns first n chars of a string (used for log-safe token prefix logging).
- `isJWTToken(s)` — returns true if string starts with `eyJ` or `eyJ0`.

---

### 3. `server/public/model/keycloak.go`

**What it is:** Data model definitions for the optional Keycloak integration.

**Contents:**
- `UserAuthServiceKeycloak = "keycloak"` — constant.
- `KeycloakTokenResponse` — response from Keycloak's `/token` endpoint.
- `KeycloakUserInfo` — response from Keycloak's `/userinfo` endpoint (sub, email, preferred_username, name, etc.).
- `KeycloakErrorResponse` — error shape from Keycloak (`error`, `error_description`).

---

### 4. `server/channels/app/keycloak.go`

**What it is:** Keycloak authentication provider — an alternative to Taruvi for password-based login.

**Key components:**

- **`KeycloakInterface`** — interface with `AuthenticateUser(rctx, username, password)`.
- **`KeycloakProvider`** — struct holding `App` reference.
- **`App.Keycloak()`** — factory method returning a `KeycloakProvider`.

**`AuthenticateUser`** (line 33) — sends a `grant_type=password` form POST to `{ServerURL}/realms/{Realm}/protocol/openid-connect/token`. On HTTP 200, returns a `KeycloakUserInfo` with just the email set (the MM user is looked up by `loginId` separately, so full userinfo is not needed). On failure, parses and logs the Keycloak error response.

---

### 5. `autologin.html`

**What it is:** A static HTML bridge page served at `/autologin` (copied into the Docker image at `/mattermost/client/autologin.html`). It enables cross-origin iframe-based SSO from a Taruvi web app into Mattermost.

**How it works:**
1. The page listens for `postMessage` events from parent frames whose origin ends in `.taruvi.space` or `.taruvi.app`.
2. On receiving a `{type: "mm-login", loginId, token}` message, it shows an "Open Chat" button.
3. When clicked (or immediately if `requestStorageAccess` is not needed), it POSTs to `/api/v4/users/login` with `auth_type: "session"` and the received session token as the password.
4. On success, redirects to `/`. On failure, posts `{type: "mm-login-failed"}` back to the parent.
5. On load, posts `{type: "mm-bridge-ready"}` to the parent to signal readiness.

**Why the button exists:** Browsers require a user gesture before granting storage access (cookies) in a cross-origin iframe. The button satisfies that requirement via `document.requestStorageAccess()`.

---

### 6. `SETUP.md`

**What it is:** Developer setup guide for the Taruvi-Mattermost integration. Documents `make` commands, environment variables, and the Taruvi-specific configuration options.

---

### 7. `DYNAMIC_ENV_INJECTION.md`

**What it is:** Architecture decision record (ADR) documenting the solution for injecting Infisical secrets into Kubernetes Operator-managed Mattermost pods. Explains why standard approaches failed and how the final solution (patching the deployment with `envFrom`) works.

---

### 8. `debug-env-issue.sh`

**What it is:** A one-off debugging script for Kubernetes. Checks the `mm-infisical` secret, the deployment's `envFrom` config, and the running pod's environment variables to diagnose why Taruvi env vars were reverting to localhost defaults.

---

## Modified Files

### 9. `server/channels/api4/user.go`

**Original purpose:** HTTP handler for the `/api/v4/users/login` endpoint.

**Taruvi changes:**

- **`auth_type` extraction** — `auth_type` is now read from the login request body and passed as a new parameter to `AuthenticateUserForLogin`. This is what allows the `autologin.html` bridge to signal a session-token login.
- **`SameSite=None` on device ID cookie** — the `CheckEmbeddedCookie` guard was removed from `attachDeviceId`. `SameSite=None` is now set on the device ID cookie for **all HTTPS requests**, not just embedded ones.
- **`loginCWS` signature update** — the internal CWS login path also updated to pass an empty `authType` to match the new `AuthenticateUserForLogin` signature.

---

### 10. `server/channels/app/authentication.go`

**Taruvi changes:**

- The `CheckPasswordAndAllCriteria` call inside `authenticateUser` is **commented out entirely**. This is a second place (in addition to `login.go`) where Mattermost's native password check is bypassed, ensuring no code path can fall back to local password validation.

---

### 11. `server/channels/app/limits.go`

**Taruvi changes:**

- `maxUsersLimit` raised from **200 → 30,000**.
- `maxUsersHardLimit` raised from **250 → 30,000**.

This removes the free-tier user cap so Taruvi deployments are not artificially limited.

---

### 12. `server/channels/utils/api.go`

**Taruvi changes:**

- `CheckOrigin` enhanced to support **wildcard subdomain matching**. Previously only exact origin matches were allowed. Now an allowed origin like `*.taruvi.space` will match any subdomain (e.g. `app.taruvi.space`, `dev.taruvi.space`). This is needed for CORS to work across Taruvi's multi-tenant subdomain setup.

---

### 13. `server/channels/web/handlers.go`

**Taruvi changes:**

- `X-Frame-Options: SAMEORIGIN` is now **conditionally omitted** when `ServiceSettings.FrameAncestors` is configured. This allows Mattermost to be embedded in an iframe from Taruvi origins without the browser blocking it. The `autologin.html` bridge depends on this change.

---

### 14. `server/build/Dockerfile`

**Taruvi changes:**

- The inner server Dockerfile (used for the dev/test environment at `server/build/Dockerfile`) is changed from `gcr.io/distroless/base-debian12` to `ubuntu:noble` with `bash` and `coreutils` installed. This mirrors the root `Dockerfile` change and ensures shell access is available in all build contexts.

---

### 15. `server/channels/app/login.go`

**Original purpose:** Mattermost's core login logic — `AuthenticateUserForLogin`, `GetUserForLogin`, `DoLogin`, session cookie helpers.

**Taruvi changes:**

#### New helper function: `hashTaruviUsername` (lines 32–38)
```go
func hashTaruviUsername(username string) string {
    sha256Hash := sha256.Sum256([]byte(username))
    sha256Hex := hex.EncodeToString(sha256Hash[:])
    md5Hash := md5.Sum([]byte(sha256Hex))
    return hex.EncodeToString(md5Hash[:])
}
```
Hashes a Taruvi username with SHA256 then MD5. **Note: this function is defined but not currently called** — it was likely used in an earlier version where Mattermost usernames were derived from Taruvi usernames.

#### Replaced authentication block in `AuthenticateUserForLogin` (lines ~85–120)
The original Mattermost code called `checkPasswordAndAllCriteria` (native password check). This is replaced with:

```
if authType == "session" || Keycloak is disabled:
    → call authenticateWithTaruvi(loginId, password, authType)
else:
    → call Keycloak().AuthenticateUser(loginId, password)
```

After external auth succeeds, the code falls through to `GetUserForLogin` to look up the pre-existing Mattermost user record by `loginId`. This means:
- Mattermost's own password is never checked.
- The user must already exist in Mattermost's database.
- Failed attempts counter is reset on success.

**Structural change:** `GetUserForLogin` is now called **after** external auth (not before as in the original). `checkUserNotBot`, `checkUserNotDisabled`, and `FailedAttempts` reset are now done inline in `AuthenticateUserForLogin` rather than delegated to `authenticateUser`.

#### New private method: `authenticateWithTaruvi` (lines ~130–155)
Calls `a.Taruvi().AuthenticateUser(...)` and then validates that the `loginId` matches either the email or username returned by Taruvi. Returns an error if they don't match (prevents one user from logging in as another).

#### `GetUserForLogin` (lines ~160–195)
Simplified from the original: `EnableUsername` and `EnableEmail` are hardcoded to `true` (instead of reading from config). LDAP fallback is preserved.

#### `AttachSessionCookies` — `SameSite=None` on all HTTPS requests
The `CheckEmbeddedCookie` guard was removed. `SameSite=None` is now set on the **session, user, and CSRF cookies** for all HTTPS requests (not just embedded ones). This is required for cross-origin iframe cookie access from the `autologin.html` bridge.

---

### 16. `server/channels/app/login_test.go`

**Taruvi changes:**

- Two existing test calls to `AuthenticateUserForLogin` updated to pass the new empty `authType` parameter (`""`), keeping the test suite compiling after the signature change.

---

### 17. `server/cmd/mmctl/commands/ldap_e2e_test.go`

**Taruvi changes:**

- One existing e2e test call to `AuthenticateUserForLogin` updated to pass the new empty `authType` parameter (`""`), keeping the mmctl e2e test compiling after the signature change.

---

### 18. `server/docker-compose.yaml`

**Taruvi changes:**

- Removed `follower` and `follower2` HA cluster services — Taruvi runs single-node only.
- Removed `enterprise` volume mount from the `leader` service (no enterprise repo dependency).
- Removed `user: ${CURRENT_UID}` from the `leader` service.
- Removed `follower`/`follower2` health conditions from the `haproxy` `depends_on` block.

---

### 19. `server/public/model/config.go`

**Taruvi changes:** Two new settings structs added to the `Config` struct.

#### `KeycloakSettings` struct (lines 3055–3083)
Fields:
- `Enable *bool` — toggles Keycloak auth on/off.
- `ServerURL *string` — base URL of the Keycloak server.
- `Realm *string` — Keycloak realm name.
- `ClientID *string` — OAuth2 client ID.
- `ClientSecret *string` — OAuth2 client secret.
- `ConnectionTimeout *int` — HTTP timeout in seconds (default: 10).

`SetDefaults()` initializes all fields to empty strings / false / 10.

#### `TaruviSettings` struct (lines 3085–3150)
Fields:
- `Enable *bool` — toggles Taruvi auth (default: false).
- `TaruviServerURL *string` — base URL of the Taruvi backend (env: `MM_TARUVISETTINGS_TARUVISERVERURL`, default: `http://localhost:8000`).
- `AuthEndpoint *string` — login endpoint path (env: `MM_TARUVISETTINGS_AUTHENDPOINT`, default: `/api/_allauth/browser/v1/auth/login`).
- `UserEndpoint *string` — user info endpoint path (env: `MM_TARUVISETTINGS_USERENDPOINT`, default: `/api/users/me/`).
- `VerifyEndpoint *string` — JWT verify endpoint path (env: `MM_TARUVISETTINGS_VERIFYENDPOINT`, default: `/api/auth/jwt/token/verify/`).
- `SessionEndpoint *string` — session validation endpoint path (env: `MM_TARUVISETTINGS_SESSIONENDPOINT`, default: `/_allauth/app/v1/auth/session`).
- `ConnectionTimeout *int` — HTTP timeout in seconds (default: 10).
- `OverrideHost *bool` — whether to set a custom `Host` header (default: false).
- `HostOverrideValue *string` — value for the `Host` header when override is enabled.

`SetDefaults()` reads each value from the corresponding `MM_TARUVISETTINGS_*` environment variable, falling back to hardcoded defaults.

#### `Config` struct additions (lines 4060–4061, 4149–4150)
```go
KeycloakSettings  KeycloakSettings
TaruviSettings    TaruviSettings
```
Both are registered in `Config.SetDefaults()`.

---

### 20. `Makefile`

**What it is:** Replaces the original Mattermost Makefile with a simplified developer workflow for the Taruvi fork.

**Taruvi-specific targets:**
- `setup` — copies `.env.example` → `.env`, runs `docker compose build`.
- `connect-to-taruvi` — connects the running `mattermost-server` container to the `taruvi_default` and `taruvi_web` Docker networks so it can reach the Taruvi backend by container name.
- `dev` — starts `docker compose up -d` then runs the webapp dev server.
- Standard targets: `stop`, `restart`, `build-server`, `restart-server`, `build-webapp`, `restart-webapp`, `install-webapp`, `logs`, `logs-server`, `clean`, `prune`.

---

### 21. `docker-compose.yml`

**What it is:** Replaces the original with a Taruvi-specific compose file.

**Taruvi-specific additions:**
- `mattermost-server` service builds from the local `Dockerfile` (custom build with Taruvi auth).
- `env_file: .env` — loads all `MM_TARUVISETTINGS_*` and `MM_KEYCLOAKSETTINGS_*` vars.
- `networks` — the `mattermost-server` service is attached to both `default` and `taruvi_default` (external) networks, allowing it to reach the Taruvi backend container.
- `taruvi_default` network declared as `external: true`.

---

### 22. `.env.example`

**What it is:** Template environment file documenting all Taruvi and Keycloak configuration variables.

**Taruvi-specific variables:**
```
MM_KEYCLOAKSETTINGS_ENABLE=true          # Keycloak takes priority when enabled
MM_KEYCLOAKSETTINGS_SERVERURL=...
MM_KEYCLOAKSETTINGS_REALM=...
MM_KEYCLOAKSETTINGS_CLIENTID=...
MM_KEYCLOAKSETTINGS_CLIENTSECRET=...
MM_KEYCLOAKSETTINGS_CONNECTIONTIMEOUT=10

MM_TARUVISETTINGS_ENABLE=false           # Fallback when Keycloak is disabled
MM_TARUVISETTINGS_TARUVISERVERURL=http://taruvi_web:8000/sites/dev-appbuild
MM_TARUVISETTINGS_AUTHENDPOINT=/_allauth/app/v1/auth/login
MM_TARUVISETTINGS_USERENDPOINT=/api/users/me/
MM_TARUVISETTINGS_VERIFYENDPOINT=/api/cloud/auth/jwt/token/verify/
MM_TARUVISETTINGS_SESSIONENDPOINT=/_allauth/app/v1/auth/session
MM_TARUVISETTINGS_CONNECTIONTIMEOUT=10
MM_TARUVISETTINGS_OVERRIDEHOST=false
MM_TARUVISETTINGS_HOSTOVERRIDEVALUE=localhost:8000
```

---

### 23. `Dockerfile`

**What it is:** Custom multi-stage Dockerfile replacing the upstream one.

**Taruvi-specific changes:**
- **Stage 1 (builder):** Builds the Go server binary from source (includes all Taruvi auth code).
- **Stage 2 (release):** Downloads the official Mattermost release package to get the pre-built webapp, config templates, and assets.
- **Stage 3 (runtime):** Uses `ubuntu:noble` (not distroless) for shell access. Copies the custom server binary over the released one. **Copies `autologin.html` into `/mattermost/client/autologin.html`** so it is served as a static file.

---

### 24. `.github/workflows/harbor-push.yml`

**What it is:** CI/CD pipeline for building and deploying the Taruvi-Mattermost image.

**Taruvi-specific behavior:**
- Triggers on pushes to `mattermost-taruvi` (staging) and `mattermost-taruviprod` (production) branches.
- Fetches secrets from **Infisical** (self-hosted at `environment.eoxvantage.com`) under the `/Mattermost` path.
- Builds and pushes the Docker image to **Harbor** at `repo.eoxvantage.com/taruvi/mattermost`.
- **Staging deploy job:** Creates/updates a Kubernetes secret `mm-infisical` from all `MM_*` env vars, then patches the `mattermost-staging` deployment to use `envFrom` referencing that secret (workaround for Mattermost Operator CRD limitations — see `DYNAMIC_ENV_INJECTION.md`).
- **Production deploy job:** Same secret creation into `mm-chatprod-infisical`, then restarts `mattermost-production` deployment. Requires `production` environment approval.

---

## Authentication Flow Summary

```
POST /api/v4/users/login
  { login_id, password, auth_type? }
        │
        ▼
AuthenticateUserForLogin (login.go)
        │
        ├─ auth_type == "session"  ──────────────────────────────────────────┐
        │                                                                     │
        ├─ KeycloakSettings.Enable == false ──────────────────────────────┐  │
        │                                                                  │  │
        └─ KeycloakSettings.Enable == true                                │  │
              │                                                            │  │
              ▼                                                            │  │
        Keycloak.AuthenticateUser                                          │  │
        POST {serverURL}/realms/{realm}/protocol/openid-connect/token     │  │
              │                                                            │  │
              └──────────────────────────────────────────────────────┐    │  │
                                                                      │    │  │
                                                              Taruvi.AuthenticateUser (taruvi.go)
                                                                      │    │  │
                                                              ┌───────┴────┘  │
                                                              │               │
                                                    password starts with eyJ? │
                                                              │               │
                                                    ┌─── yes ┤               │
                                                    │         └─── no ────┐  │
                                                    │                     │  │
                                                    ▼                     ▼  ▼
                                              verifyJWTToken    authenticateWithPassword
                                              POST /verify      POST /auth/login
                                                    │
                                                    ▼
                                              getUserInfoWithToken
                                              GET /api/users/me/
                                                              │
                                                              │  (session)
                                                              ▼
                                                    validateSessionToken
                                                    GET /session (X-Session-Token header)
        │
        ▼
GetUserForLogin — look up pre-existing MM user by loginId
        │
        ▼
DoLogin — create MM session, set cookies
```

---

## Files Not Changed

All other Mattermost files (webapp, plugins, API handlers beyond the login endpoint, store layer, etc.) are **unmodified** from the upstream 11.3.0 release. The Taruvi changes are intentionally minimal and confined to the auth layer, CORS handling, iframe embedding, and deployment infrastructure.
