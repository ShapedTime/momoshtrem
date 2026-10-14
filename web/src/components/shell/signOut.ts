/** End the session, drop every cached query, then do a full navigation to /login. */
export async function signOut(clearCache: () => void): Promise<void> {
  await fetch('/api/auth', { method: 'DELETE', credentials: 'same-origin' }).catch(() => undefined);
  clearCache();
  window.location.assign('/login');
}
