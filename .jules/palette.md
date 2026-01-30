## 2024-05-23 - Dynamic ARIA Labels for State Toggles
**Learning:** Interactive toggle buttons (like password visibility) often use static `aria-label`s like "Toggle visibility", which forces users to guess the current state.
**Action:** Always dynamically update `aria-label` to reflect the *action* that will happen (e.g., "Show password" -> "Hide password") or use `aria-pressed`/`aria-expanded` where appropriate. For icon-only buttons, ensure the icon is `aria-hidden="true"` to avoid redundant announcements.
