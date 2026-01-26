## 2024-05-23 - Centralized Input Validation Pattern
**Vulnerability:** User input (specifically usernames) was being used in database queries and logic without validation, potentially allowing for DoS (long strings) or XSS (if rendered unsafely, though Go templates handle this mostly).
**Learning:** The application lacked a centralized place for common validation logic, leading to inconsistent or missing checks in handlers.
**Prevention:** Created `utils/validation.go` to house reusable validation logic (starting with `ValidateUsername`). Future validations (email, phone, etc.) should be added there and called from handlers before any processing.

## 2025-10-26 - Unsafe HTML Construction in Handlers
**Vulnerability:** `HandleCreateGroupFromDashboard` constructed HTML responses using string concatenation (`c.SendString`) with unescaped user input (`group.Name`), leading to Stored XSS.
**Learning:** Developers might bypass template safety when sending small HTMX snippets via `SendString`.
**Prevention:** Always use `html.EscapeString` when manually building HTML, or strictly prefer `c.Render` with templates which handle escaping automatically.

## 2025-10-27 - Reflected XSS in Friend Request Handler
**Vulnerability:** The `HandleSendFriendRequest` function echoed the `targetUsername` parameter directly into the HTML response string without escaping, creating a Reflected XSS vulnerability if a user could exist with malicious characters (or if validation changes).
**Learning:** Even with input validation on creation, parameters reflected in responses must be escaped at the point of output to ensure defense in depth.
**Prevention:** Used `html.EscapeString` when constructing the HTML response manually. Also renamed `handle_firends.go` to `handle_friends.go` to fix a typo and clean up the codebase.
