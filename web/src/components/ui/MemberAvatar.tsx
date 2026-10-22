import { MEMBER_COLORS, type MemberColor } from '@/lib/api/contract';
import { memberInitial } from '@/lib/members/display';
import { cx } from '@/lib/utils/cx';

export type MemberAvatarSize = 'xs' | 'sm' | 'md' | 'lg' | 'xl';

const isMemberColor = (color: string): color is MemberColor => (MEMBER_COLORS as readonly string[]).includes(color);

/**
 * A household member's initial on a tint of their colour (#239): xs 20, sm 28,
 * md 40, lg 64, xl the picker's (its CSS sizes it). Identity, never status, and
 * always beside the member's name, so it's hidden from screen readers. A
 * colour key this web doesn't know gets the neutral gradient.
 */
export function MemberAvatar({ name, color, size = 'md', className }: { name: string; color: string; size?: MemberAvatarSize; className?: string }) {
  return (
    <span aria-hidden="true" className={cx('member-avatar', `member-avatar--${size}`, className)} data-color={isMemberColor(color) ? color : undefined}>
      {memberInitial(name)}
    </span>
  );
}
