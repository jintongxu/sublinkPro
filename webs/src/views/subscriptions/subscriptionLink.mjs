export function resolveSubscriptionServerUrl({ systemDomain = '', origin, development = false, apiUrl = '', hostname = '' }) {
  const configured = systemDomain.trim();
  if (configured) {
    return (/^https?:\/\//i.test(configured) ? configured : `http://${configured}`).replace(/\/+$/, '');
  }
  if (development && apiUrl) {
    try {
      const target = new URL(apiUrl, origin);
      if (['localhost', '127.0.0.1', '[::1]', '::1'].includes(target.hostname) && hostname) target.hostname = hostname;
      return target.origin;
    } catch {
      // Invalid development target: use the page origin.
    }
  }
  return origin;
}

export function selectQuickCopyShare(shares, now = new Date()) {
  const available = shares.filter((share) => {
    if (!share.enabled || !share.token) return false;
    let expires;
    if (share.expire_type === 1 && share.expire_days > 0) {
      expires = new Date(share.created_at);
      expires.setDate(expires.getDate() + share.expire_days);
    } else if (share.expire_type === 2 && share.expire_at) {
      expires = new Date(share.expire_at);
    }
    return !expires || (Number.isFinite(expires.getTime()) && now.getTime() <= expires.getTime());
  });
  return available.find((share) => share.is_legacy) || available[0] || null;
}

export function buildAutoSubscriptionUrl(serverUrl, token) {
  return `${serverUrl.replace(/\/+$/, '')}/c/?${new URLSearchParams({ token }).toString()}`;
}
