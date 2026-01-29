## 2024-05-23 - Centralized Input Validation Pattern
**Vulnerability:** User input (specifically usernames) was being used in database queries and logic without validation, potentially allowing for DoS (long strings) or XSS (if rendered unsafely, though Go templates handle this mostly).
**Learning:** The application lacked a centralized place for common validation logic, leading to inconsistent or missing checks in handlers.
**Prevention:** Created `utils/validation.go` to house reusable validation logic (starting with `ValidateUsername`). Future validations (email, phone, etc.) should be added there and called from handlers before any processing.

## 2025-10-26 - Unsafe HTML Construction in Handlers
**Vulnerability:** `HandleCreateGroupFromDashboard` constructed HTML responses using string concatenation (`c.SendString`) with unescaped user input (`group.Name`), leading to Stored XSS.
**Learning:** Developers might bypass template safety when sending small HTMX snippets via `SendString`.
**Prevention:** Always use `html.EscapeString` when manually building HTML, or strictly prefer `c.Render` with templates which handle escaping automatically.

## 2026-01-29 - Missing Login CSRF Protection
**Vulnerability:** The CSRF middleware was strictly coupled to the user session (`session_id`), causing it to skip validation for unauthenticated requests like Login and Register forms. This exposed the application to Login CSRF attacks.
**Learning:** Tying security mechanisms solely to authentication state can leave pre-authentication endpoints vulnerable. Security features like CSRF should often be session-agnostic or have a fallback for guests.
**Prevention:** Refactored CSRF logic to support a "Guest" identifier (`csrf_client_id` cookie) when no session exists, ensuring all POST requests are protected regardless of auth state.
