## 2024-05-23 - Password Visibility Toggle
**Learning:** Adding a password visibility toggle is a high-impact micro-UX improvement. It significantly reduces user frustration (typing errors) and improves accessibility for users with cognitive or motor impairments who may struggle with masked input fields. It requires minimal code but directly addresses a core usability heuristic (User Control and Freedom).
**Action:** Always check authentication forms for password masking and propose a toggle if missing. Ensure the toggle is keyboard accessible and uses ARIA labels.

## 2024-05-24 - Dynamic State Labels
**Learning:** Static ARIA labels on state-toggling buttons (like "Toggle password") force screen reader users to guess the current state. Dynamic labels ("Show password" vs "Hide password") reduce cognitive load and provide immediate confirmation of the action's result.
**Action:** When implementing toggle buttons, ensure the accessible label changes to reflect the *next* action or the *current* state explicitly, rather than just describing the button's general function.
