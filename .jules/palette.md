## 2024-05-22 - Static Labels on Dynamic Toggles
**Learning:** Interactive toggle buttons (like password visibility) often fail accessibility checks because their `aria-label` remains static while the visual state changes. A generic label like "Toggle password visibility" forces screen reader users to guess the current state.
**Action:** Use JavaScript to dynamically update `aria-label` to reflect the *action* the button will perform (e.g., "Show password" vs "Hide password") or use `aria-pressed` for simple toggles. Ensure inline SVGs are hidden from screen readers with `aria-hidden="true"`.
