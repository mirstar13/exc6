## 2024-05-23 - Centralized Input Validation Pattern
**Vulnerability:** User input (specifically usernames) was being used in database queries and logic without validation, potentially allowing for DoS (long strings) or XSS (if rendered unsafely, though Go templates handle this mostly).
**Learning:** The application lacked a centralized place for common validation logic, leading to inconsistent or missing checks in handlers.
**Prevention:** Created `utils/validation.go` to house reusable validation logic (starting with `ValidateUsername`). Future validations (email, phone, etc.) should be added there and called from handlers before any processing.

## 2024-06-03 - Login CSRF Protection
**Vulnerability:** Login and Register forms were not protected against CSRF, allowing potential Login CSRF attacks. Unauthenticated users did not have CSRF tokens.
**Learning:** CSRF protection often relies on session IDs. For unauthenticated endpoints (Login), we need a separate tracking mechanism (like a `csrf_client_id` cookie) to key the CSRF token storage.
**Prevention:** Refactored CSRF middleware to use `session_id` OR `csrf_client_id`. Applied CSRF protection globally, including for `/login` and `/register`.
