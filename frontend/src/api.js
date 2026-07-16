export const getBaseUrl = (location = window.location) => {
    const pathname = location.pathname.replace(/\/+$/, '');
    return `${location.origin}${pathname === '/' ? '' : pathname}`;
};

export const getWebSocketUrl = (baseUrl) => `${baseUrl.replace(/^http/, 'ws')}/ws/stream`;

export const readApiError = async (response) => {
    try {
        const payload = await response.json();
        if (typeof payload.detail === 'string') return payload.detail;
        if (payload.detail?.message) return payload.detail.message;
        if (payload.message) return payload.message;
    } catch {
        // The fallback below is clearer than exposing a JSON parsing failure.
    }
    return `Request failed with status ${response.status}`;
};
