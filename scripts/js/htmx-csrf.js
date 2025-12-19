/**
 * HTMX CSRF Token Configuration
 * 
 * This script automatically attaches CSRF tokens to all HTMX requests
 * that use state-changing methods (POST, PUT, DELETE, PATCH)
 * 
 * Add this to your base template's <head> section:
 * <script src="/static/js/htmx-csrf.js"></script>


(function() {
    'use strict';

    // Get CSRF token from meta tag or cookie
    function getCSRFToken() {
        // Try meta tag first (injected by server)
        const metaTag = document.querySelector('meta[name="csrf-token"]');
        if (metaTag) {
            return metaTag.getAttribute('content');
        }

        // Fallback to cookie
        const cookieMatch = document.cookie.match(/csrf_token=([^;]+)/);
        return cookieMatch ? cookieMatch[1] : null;
    }

    // Refresh CSRF token periodically (every 50 minutes for 1-hour tokens)
    function refreshCSRFToken() {
        fetch('/api/v1/csrf/refresh', {
            method: 'POST',
            credentials: 'include'
        })
        .then(response => response.json())
        .then(data => {
            if (data.csrf_token) {
                // Update meta tag
                let metaTag = document.querySelector('meta[name="csrf-token"]');
                if (!metaTag) {
                    metaTag = document.createElement('meta');
                    metaTag.name = 'csrf-token';
                    document.head.appendChild(metaTag);
                }
                metaTag.setAttribute('content', data.csrf_token);
                console.log('CSRF token refreshed');
            }
        })
        .catch(err => {
            console.error('Failed to refresh CSRF token:', err);
        });
    }

    // Attach CSRF token to HTMX requests
    document.addEventListener('htmx:configRequest', function(event) {
        const method = event.detail.verb.toUpperCase();
        
        // Only add CSRF token for state-changing methods
        if (['POST', 'PUT', 'DELETE', 'PATCH'].includes(method)) {
            const token = getCSRFToken();
            if (token) {
                event.detail.headers['X-CSRF-Token'] = token;
            } else {
                console.warn('CSRF token not found for', method, 'request');
            }
        }
    });

    // Handle CSRF errors
    document.addEventListener('htmx:responseError', function(event) {
        if (event.detail.xhr.status === 403) {
            const response = JSON.parse(event.detail.xhr.responseText || '{}');
            if (response.error && response.error.code === 'VALIDATION_FAILED') {
                console.error('CSRF validation failed, refreshing token...');
                
                // Refresh token and retry the request
                refreshCSRFToken();
                
                // Show user-friendly error
                const errorDiv = document.createElement('div');
                errorDiv.className = 'error-notification';
                errorDiv.innerHTML = `
                    <div class="bg-red-500/10 border border-red-500/30 text-red-400 p-4 rounded-xl text-sm fixed top-4 right-4 z-50 animate-slide-in">
                        <p>Security token expired. Please try again.</p>
                    </div>
                `;
                document.body.appendChild(errorDiv);
                
                // Remove after 5 seconds
                setTimeout(() => errorDiv.remove(), 5000);
            }
        }
    });

    // Initialize: Refresh token every 50 minutes (for 1-hour expiration)
    setInterval(refreshCSRFToken, 50 * 60 * 1000);

    // Also refresh when page becomes visible again (user returned to tab)
    document.addEventListener('visibilitychange', function() {
        if (!document.hidden) {
            refreshCSRFToken();
        }
    });

    console.log('HTMX CSRF protection initialized');
})();
 */