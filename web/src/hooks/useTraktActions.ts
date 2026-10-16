'use client';

import { type QueryClient, useMutation, useMutationState, useQueryClient } from '@tanstack/react-query';
import { libraryKeys, systemKeys, titleKeys, traktApi, traktKeys } from '@/lib/api/endpoints';
import type { TraktConnect, TraktIntegration, TraktSettings, TraktSettingsPatch } from '@/lib/api/contract';

/**
 * What Settings › Trakt and the ⌘K palette do to the integration (PLAN §6
 * "Trakt (B5)"). Each answer goes into the integration's cache, so the
 * screen moves at once; useTraktIntegration's polling takes it from there.
 */

const SETTINGS_MUTATION = ['integrations', 'trakt', 'settings'] as const;

const setIntegration = (queryClient: QueryClient, update: (current: TraktIntegration) => TraktIntegration) =>
  queryClient.setQueryData<TraktIntegration>(traktKeys.integration, (current) => (current ? update(current) : current));

/** A device code (POST /connect): the screen shows it straight away and polling starts. */
export function useTraktConnect() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: traktApi.connect,
    onSuccess: (code: TraktConnect) => {
      // No refetch here: the backend checks Trakt every `interval` seconds and the integration polls every 3
      setIntegration(queryClient, (current) => ({ ...current, state: 'pending', connected: false, pending: code, connect_error: null }));
    },
  });
}

/** Sync now (POST /sync, 202): starts one or joins the running one. */
export function useTraktSync() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: traktApi.sync,
    onSuccess: (integration) => queryClient.setQueryData(traktKeys.integration, integration),
  });
}

/**
 * DELETE /integrations/trakt: cancels a pending code, or disconnects and
 * purges what was imported, so /me, the watch state on titles and the
 * library, and /status all change.
 */
export function useTraktDisconnect() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: traktApi.disconnect,
    onSuccess: () =>
      Promise.all([
        queryClient.invalidateQueries({ queryKey: traktKeys.integration }),
        queryClient.invalidateQueries({ queryKey: traktKeys.me }),
        queryClient.invalidateQueries({ queryKey: titleKeys.all }),
        queryClient.invalidateQueries({ queryKey: libraryKeys.all }),
        queryClient.invalidateQueries({ queryKey: systemKeys.status }),
      ]),
  });
}

/**
 * The three switches (PUT /settings). They move at once: the screen shows
 * the saved settings with the changes still on their way laid over them
 * (usePendingTraktSettings), so a refused change simply falls back.
 * Up next reads two of them.
 */
export function useTraktSettings() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: SETTINGS_MUTATION,
    mutationFn: (patch: TraktSettingsPatch) => traktApi.updateSettings(patch),
    onSuccess: async (settings) => {
      // A poll that left before this save would bring the old values back
      await queryClient.cancelQueries({ queryKey: traktKeys.integration });
      setIntegration(queryClient, (current) => ({ ...current, settings }));
      void queryClient.invalidateQueries({ queryKey: traktKeys.me });
    },
  });
}

/** The settings as they'll be once every change on its way lands (later changes win). */
export function usePendingTraktSettings(saved: TraktSettings | undefined): TraktSettings | undefined {
  const patches = useMutationState({
    filters: { mutationKey: SETTINGS_MUTATION, status: 'pending' },
    select: (mutation) => mutation.state.variables as TraktSettingsPatch | undefined,
  });
  return saved && Object.assign({}, saved, ...patches);
}
