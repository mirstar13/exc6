/**
 * HTMX CSRF Token Configuration
 * 
 * This script automatically attaches CSRF tokens to all HTMX requests
 * that use state-changing methods (POST, PUT, DELETE, PATCH)
 */

(function() {
    'use strict';

    // Get CSRF token from meta tag, hidden input, or cookie
    function getCSRFToken() {
        // Priority 1: Meta tag (most reliable)
        const metaTag = document.querySelector('meta[name="csrf-token"]');
        if (metaTag) {
            const token = metaTag.getAttribute('content');
            if (token) {
                console.log('CSRF: Found token in meta tag');
                return token;
            }
        }

        // Priority 2: Hidden input in current form context
        const hiddenInput = document.querySelector('input[name="csrf_token"]');
        if (hiddenInput && hiddenInput.value) {
            console.log('CSRF: Found token in hidden input');
            return hiddenInput.value;
        }

        // Priority 3: Cookie (fallback)
        const cookieMatch = document.cookie.match(/csrf_token=([^;]+)/);
        if (cookieMatch) {
            console.log('CSRF: Found token in cookie');
            return cookieMatch[1];
        }

        console.warn('CSRF: No token found!');
        return null;
    }

    // Attach CSRF token to HTMX requests
    document.addEventListener('htmx:configRequest', function(event) {
        const method = event.detail.verb.toUpperCase();
        
        // Only add CSRF token for state-changing methods
        if (['POST', 'PUT', 'DELETE', 'PATCH'].includes(method)) {
            const token = getCSRFToken();
            if (token) {
                event.detail.headers['X-CSRF-Token'] = token;
                console.log('CSRF: Attached token to', method, 'request to', event.detail.path);
            } else {
                console.error('CSRF: Token not found for', method, 'request to', event.detail.path);
            }
        }
    });

    // Handle CSRF errors
    document.addEventListener('htmx:responseError', function(event) {
        if (event.detail.xhr.status === 403) {
            try {
                const response = JSON.parse(event.detail.xhr.responseText || '{}');
                if (response.error && response.error.code === 'VALIDATION_FAILED') {
                    console.error('CSRF validation failed');
                    
                    // Show user-friendly error
                    const errorDiv = document.createElement('div');
                    errorDiv.innerHTML = `
                        <div class="bg-red-500/10 border border-red-500/30 text-red-400 p-4 rounded-xl text-sm fixed top-4 right-4 z-50" style="animation: slideIn 0.3s ease-out;">
                            <p><strong>Security Error:</strong> Your session may have expired. Please refresh the page.</p>
                        </div>
                    `;
                    document.body.appendChild(errorDiv);
                    
                    // Remove after 5 seconds
                    setTimeout(() => errorDiv.remove(), 5000);
                }
            } catch (e) {
                console.error('Error parsing CSRF error response:', e);
            }
        }
    });

    // Debug: Log when HTMX makes requests
    document.addEventListener('htmx:beforeRequest', function(event) {
        console.log('HTMX: Making request to', event.detail.path);
    });

    console.log('HTMX CSRF protection initialized');
})();