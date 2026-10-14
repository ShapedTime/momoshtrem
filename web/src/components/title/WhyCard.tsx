import { CatAvatar } from '@/components/brand';

/** "Why it's for you" under the hero text; nothing without a recommendation reason. */
export function WhyCard({ reason }: { reason: string | null }) {
  if (!reason) return null;
  return (
    <div className="why-card">
      <CatAvatar size={28} />
      <p>
        <span className="t-micro">Why it’s for you</span>
        <br />
        {reason}
      </p>
    </div>
  );
}
