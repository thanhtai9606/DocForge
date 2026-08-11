# DocForge Web

React + TypeScript + Vite frontend (Phase 4).

## Auth

SSO only:

- Google OAuth
- Microsoft Entra ID (Azure AD) OAuth

Configure on the API:

```bash
GOOGLE_OAUTH_CLIENT_ID=...
GOOGLE_OAUTH_CLIENT_SECRET=...
MICROSOFT_OAUTH_CLIENT_ID=...
MICROSOFT_OAUTH_CLIENT_SECRET=...
MICROSOFT_OAUTH_TENANT=common
WEB_ORIGIN=http://localhost:5173
API_PUBLIC_ORIGIN=http://localhost:8080
AUTH_SECRET=replace-me
AUTH_BYPASS=0
```

Redirect URIs to register with IdPs:

- `http://localhost:8080/api/v1/auth/google/callback`
- `http://localhost:8080/api/v1/auth/microsoft/callback`

Local development without IdP credentials can set `AUTH_BYPASS=1`.

## Develop

```bash
npm install
npm run dev
```

API proxy: Vite forwards `/api`, `/healthz`, `/metrics` to `http://localhost:8080`.
