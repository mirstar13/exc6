## 2024-05-23 - Password Visibility Toggle
**Learning:** Adding a password visibility toggle is a high-impact micro-UX improvement. It significantly reduces user frustration (typing errors) and improves accessibility for users with cognitive or motor impairments who may struggle with masked input fields. It requires minimal code but directly addresses a core usability heuristic (User Control and Freedom).
**Action:** Always check authentication forms for password masking and propose a toggle if missing. Ensure the toggle is keyboard accessible and uses ARIA labels.

## 2024-05-24 - Icon-Only Button Accessibility Pattern
**Learning:** Icon-only buttons (like sidebar toggles, notification bells) are frequently missing accessible names in this codebase. While they often have `title` attributes, they lack `aria-label` for screen readers and `aria-expanded` for state indication. This creates a significant barrier for blind users who rely on screen readers.
**Action:** Systematically audit icon-only buttons. Prefer `aria-label` over `title` for accessibility. Ensure state-changing buttons (expand/collapse) have programmatic state indicators (`aria-expanded`) and update them via JS.
