## 2024-05-21 - Accessible Sidebar Navigation
**Learning:** Icon-only buttons in the sidebar (notifications, friends, logout) were missing `aria-label`s, making them inaccessible to screen readers. Also, there was no "Skip to main content" link, forcing keyboard users to tab through the entire sidebar to reach the chat.
**Action:** Always verify icon-only buttons have descriptive `aria-label`s and ensure a skip link is present for main content areas. Use `focus-visible` to provide clear visual feedback for keyboard navigation.
