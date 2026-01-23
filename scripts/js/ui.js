function togglePasswordVisibility(button) {
    const container = button.closest('.relative');
    const input = container.querySelector('input');
    const eyeOpen = button.querySelector('.eye-open');
    const eyeClosed = button.querySelector('.eye-closed');

    if (input.type === 'password') {
        input.type = 'text';
        eyeOpen.classList.add('hidden');
        eyeClosed.classList.remove('hidden');
        button.setAttribute('aria-label', 'Hide password');
    } else {
        input.type = 'password';
        eyeOpen.classList.remove('hidden');
        eyeClosed.classList.add('hidden');
        button.setAttribute('aria-label', 'Show password');
    }
}
