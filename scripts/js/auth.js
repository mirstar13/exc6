/**
 * Auth utilities for SecureChat
 */

/**
 * Toggles password visibility for a given button context.
 * Expects the button to be within a container that also has the input.
 * Updates aria-label for accessibility.
 * @param {HTMLButtonElement} btn - The toggle button
 */
function togglePassword(btn) {
    const container = btn.parentElement;
    const input = container.querySelector('input');

    if (!input) return;

    const isPassword = input.getAttribute('type') === 'password';
    const newType = isPassword ? 'text' : 'password';
    input.setAttribute('type', newType);

    // Update ARIA label
    // If visible (text), action is to Hide. If hidden (password), action is to Show.
    const action = isPassword ? 'Hide' : 'Show';
    btn.setAttribute('aria-label', `${action} password`);

    // Toggle icons
    const eyeIcon = btn.querySelector('.eye-icon');
    const eyeOffIcon = btn.querySelector('.eye-off-icon');

    if (eyeIcon && eyeOffIcon) {
        if (newType === 'text') {
            eyeIcon.classList.add('hidden');
            eyeOffIcon.classList.remove('hidden');
        } else {
            eyeIcon.classList.remove('hidden');
            eyeOffIcon.classList.add('hidden');
        }
    }
}

// Make globally available
window.togglePassword = togglePassword;
