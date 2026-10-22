'use client';

import { useEffect, useState } from 'react';
import { LogOut, RefreshCw } from 'lucide-react';
import { Mark } from '@/components/brand';
import { useSignOut } from '@/components/shell/signOut';
import { Banner, Button, MemberAvatar, Spinner } from '@/components/ui';
import type { Member } from '@/lib/api/contract';
import { currentUrl, useSwitchMember } from '@/lib/members/switch';
import { cx } from '@/lib/utils/cx';

export const WHO_TITLE = 'Who’s watching?';

interface PersonTileProps {
  member: Pick<Member, 'name' | 'color'>;
  /** Picked: the page is on its way to their For You. */
  busy?: boolean;
  /** Someone else was picked. */
  disabled?: boolean;
  onPick?: () => void;
  className?: string;
}

/** One person on the picker: their avatar and name; the button's name is theirs. */
export function PersonTile({ member, busy = false, disabled = false, onPick, className }: PersonTileProps) {
  return (
    <button type="button" className={cx('who-tile', className)} aria-busy={busy || undefined} disabled={disabled} onClick={onPick}>
      <span className="who-avatar">
        <MemberAvatar name={member.name} color={member.color} size="xl" />
        {busy && (
          <span className="who-spinner">
            <Spinner size={28} />
          </span>
        )}
      </span>
      <span className="who-name">{member.name}</span>
    </button>
  );
}

type WhoIsWatchingProps =
  | { members: Member[]; failed?: undefined }
  /** GET /members failed (not a 404): say so, with Try again. */
  | { members?: undefined; failed: { onRetry: () => void; retrying: boolean } };

/**
 * "Who's watching?" (#239): the household's people as big tiles, before the
 * app (no top bar or tab bar). Picking someone keeps the URL, so a deep link
 * opened on a new device lands where it was going.
 */
export function WhoIsWatching({ members, failed }: WhoIsWatchingProps) {
  const switchTo = useSwitchMember();
  const signOut = useSignOut();
  const [picked, setPicked] = useState<number | null>(null);

  useEffect(() => {
    const previous = document.title;
    document.title = `${WHO_TITLE} · momoshtrem`;
    return () => {
      document.title = previous;
    };
  }, []);

  const pick = (id: number) => {
    if (picked !== null) return;
    setPicked(id);
    switchTo(id, currentUrl());
  };

  return (
    <main id="main" tabIndex={-1} className="who outline-none">
      <div className="who-brand">
        <Mark size={26} />
        <span>momoshtrem</span>
      </div>
      <div className="who-body">
        <h1 className="who-title t-title-1">{WHO_TITLE}</h1>
        {failed ? (
          <Banner
            tone="error"
            role="alert"
            className="who-error"
            title="Couldn’t load who’s watching"
            action={
              <Button size="sm" loading={failed.retrying} onClick={failed.onRetry}>
                <RefreshCw size={16} aria-hidden="true" /> Try again
              </Button>
            }
          >
            momoshtrem didn’t answer. Check that it’s running, then try again.
          </Banner>
        ) : (
          <ul className="who-tiles" aria-label="People">
            {members.map((member) => (
              <li key={member.id}>
                <PersonTile member={member} busy={picked === member.id} disabled={picked !== null && picked !== member.id} onPick={() => pick(member.id)} />
              </li>
            ))}
          </ul>
        )}
      </div>
      <Button variant="ghost" className="who-foot" onClick={() => void signOut()}>
        <LogOut size={18} aria-hidden="true" /> Log out
      </Button>
    </main>
  );
}
