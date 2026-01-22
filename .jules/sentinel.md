## 2024-05-23 - Centralized Input Validation Pattern
**Vulnerability:** User input (specifically usernames) was being used in database queries and logic without validation, potentially allowing for DoS (long strings) or XSS (if rendered unsafely, though Go templates handle this mostly).
**Learning:** The application lacked a centralized place for common validation logic, leading to inconsistent or missing checks in handlers.
**Prevention:** Created `utils/validation.go` to house reusable validation logic (starting with `ValidateUsername`). Future validations (email, phone, etc.) should be added there and called from handlers before any processing.

## 2024-05-24 - Unused Security Middleware
**Vulnerability:** Public authentication endpoints (`/login`, `/register`) lacked rate limiting, exposing them to brute force attacks.
**Learning:** A fully functional `limiter` middleware existed in `server/middleware/limiter` but was completely unused in the application routes. Dead code can sometimes be a sign of forgotten security implementations.
**Prevention:** Wired up the existing rate limiter to the public routes, utilizing Redis for distributed state storage.
