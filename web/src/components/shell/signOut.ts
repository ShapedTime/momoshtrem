import { useCallback } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { useToast } from '@/components/ui';
import { apiFetch } from '@/lib/api/client';

/**
 * End the session, drop every cached query, then do a full navigation to
 * /login. Resolves false (and stays put) when the server didn't sign you out:
 * /login would only bounce a still-valid session back into the app.
 */
export async function signOut(clearCache: () => void): Promise<boolean> {
  try {
    const response = await apiFetch('/api/auth', { method: 'DELETE' });
    if (!response.ok) return false;
  } catch {
    return false;
  }
  clearCache();
  window.location.assign('/login');
  return true;
}

/** `signOut` for the shell's menus: clears this app's cache and says so when it fails. */
export function useSignOut(): () => Promise<void> {
  const queryClient = useQueryClient();
  const { toast } = useToast();
  return useCallback(async () => {
    if (await signOut(() => queryClient.clear())) return;
    toast({ tone: 'error', title: 'Couldn’t sign out', body: 'Check your connection and try again.' });
  }, [queryClient, toast]);
}
