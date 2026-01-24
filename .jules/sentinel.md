## 2024-05-23 - Centralized Input Validation Pattern
**Vulnerability:** User input (specifically usernames) was being used in database queries and logic without validation, potentially allowing for DoS (long strings) or XSS (if rendered unsafely, though Go templates handle this mostly).
**Learning:** The application lacked a centralized place for common validation logic, leading to inconsistent or missing checks in handlers.
**Prevention:** Created `utils/validation.go` to house reusable validation logic (starting with `ValidateUsername`). Future validations (email, phone, etc.) should be added there and called from handlers before any processing.

## 2024-05-24 - Rate Limiting on Auth Endpoints
**Vulnerability:** The `/login` and `/register` endpoints lacked specific rate limiting, relying only on the global rate limiter (200 req/s), which is insufficient to prevent brute-force attacks on user credentials.
**Learning:** Publicly accessible authentication endpoints require much stricter rate limits than the rest of the API to prevent credential stuffing and brute-force attacks.
**Prevention:** Implemented a specific Redis-backed token bucket limiter for `/login` and `/register` routes (5 attempts/minute) in `server/routes/public.go`.
