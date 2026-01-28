## 2024-05-23 - Dynamic ARIA Labels for State Toggles
**Learning:** Static `aria-label` attributes on toggle buttons (like "Show/Hide Password") can confuse screen reader users if they don't reflect the current state.
**Action:** Use JavaScript to dynamically update `aria-label` (e.g., from "Show password" to "Hide password") when the state changes, ensuring the label always describes the *next* action or current state accurately.
