## 2024-05-23 - Centralized Input Validation Pattern
**Vulnerability:** User input (specifically usernames) was being used in database queries and logic without validation, potentially allowing for DoS (long strings) or XSS (if rendered unsafely, though Go templates handle this mostly).
**Learning:** The application lacked a centralized place for common validation logic, leading to inconsistent or missing checks in handlers.
**Prevention:** Created `utils/validation.go` to house reusable validation logic (starting with `ValidateUsername`). Future validations (email, phone, etc.) should be added there and called from handlers before any processing.

## 2025-10-26 - Unsafe HTML Construction in Handlers
**Vulnerability:** `HandleCreateGroupFromDashboard` constructed HTML responses using string concatenation (`c.SendString`) with unescaped user input (`group.Name`), leading to Stored XSS.
**Learning:** Developers might bypass template safety when sending small HTMX snippets via `SendString`.
**Prevention:** Always use `html.EscapeString` when manually building HTML, or strictly prefer `c.Render` with templates which handle escaping automatically.

## 2025-10-27 - User Enumeration via Timing Attacks
**Vulnerability:** Authentication handlers (e.g., `HandleUserLogin`) returned immediately when a user was not found (`sql.ErrNoRows`), but performed an expensive `bcrypt` comparison when a user was found. This timing discrepancy allowed attackers to enumerate valid usernames.
**Learning:** Security-sensitive operations like login must execute in constant time (or indistinguishable time) regardless of the input's validity.
**Prevention:** Always perform the expensive operation (hash comparison) even if the user is not found, using a pre-calculated dummy hash to ensure consistent timing.
