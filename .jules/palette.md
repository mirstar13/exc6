## 2024-10-24 - Accessibility of Icon-Only Buttons
**Learning:** Icon-only buttons often rely on `title` attributes for tooltips, but this is insufficient for screen reader accessibility. `aria-label` provides a robust, accessible name for these interactive elements.
**Action:** Always verify that icon-only buttons (and inputs without visible labels) have an explicit `aria-label` attribute. `title` can be kept for mouse hover tooltips, but `aria-label` is mandatory for a11y.
