import { Button, Chip, MemberAvatar, StatusPill } from '@/components/ui';
import type { Member } from '@/lib/api/contract';
import { memberTraktLine } from '@/lib/members/display';

interface MemberRowProps {
  member: Member;
  /** Who's watching on this device. */
  current: boolean;
  onEdit: () => void;
}

/** One person in Settings › Household (#247): who they are, whether it's you, their Trakt in a line, and Edit. */
export function MemberRow({ member, current, onEdit }: MemberRowProps) {
  return (
    <li className="set-member">
      <MemberAvatar name={member.name} color={member.color} size="md" />
      <div className="set-member-text">
        <span className="set-member-name">
          <strong>{member.name}</strong>
          {current && <Chip tone="hi">Watching now</Chip>}
          {member.trakt.state === 'reauth' && <StatusPill status="warning" label="Sign in again" />}
        </span>
        <span className="t-small text-fg-3">{memberTraktLine(member)}</span>
      </div>
      <Button variant="ghost" size="sm" className="-mr-3 shrink-0" aria-label={`Edit ${member.name}`} onClick={onEdit}>
        Edit
      </Button>
    </li>
  );
}
