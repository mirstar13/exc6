## 2024-05-23 - Centralized Input Validation Pattern
**Vulnerability:** User input (specifically usernames) was being used in database queries and logic without validation, potentially allowing for DoS (long strings) or XSS (if rendered unsafely, though Go templates handle this mostly).
**Learning:** The application lacked a centralized place for common validation logic, leading to inconsistent or missing checks in handlers.
**Prevention:** Created `utils/validation.go` to house reusable validation logic (starting with `ValidateUsername`). Future validations (email, phone, etc.) should be added there and called from handlers before any processing.

## 2025-10-26 - Unsafe HTML Construction in Handlers
**Vulnerability:** `HandleCreateGroupFromDashboard` constructed HTML responses using string concatenation (`c.SendString`) with unescaped user input (`group.Name`), leading to Stored XSS.
**Learning:** Developers might bypass template safety when sending small HTMX snippets via `SendString`.
**Prevention:** Always use `html.EscapeString` when manually building HTML, or strictly prefer `c.Render` with templates which handle escaping automatically.

## 2025-10-27 - DOM XSS in UI Components
**Vulnerability:** The `showToast` method in `websocket-client.js` used `innerHTML` to render dynamic content (title, subtitle), allowing potential XSS via malicious WebSocket messages (e.g., call notifications).
**Learning:** Client-side UI components that handle user input (even from "trusted" sources like the backend/peers) must avoid `innerHTML` or template literals that interpolate strings directly into HTML.
**Prevention:** Use `document.createElement()` and `textContent` for text content. If rich text is required, use a sanitizer library or strictly whitelisted parsing.
