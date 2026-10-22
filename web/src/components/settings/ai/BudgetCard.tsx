import { RefreshCw } from 'lucide-react';
import { Banner, Button, MemberAvatar } from '@/components/ui';
import type { AiUsage } from '@/lib/api/contract';
import { budgetMonth, type MemberSpend, spendBar, spendLine, spendProjection, usd } from '@/lib/settings/ai';
import { cx } from '@/lib/utils/cx';
import { rowDescribedBy, SettingRow, SettingsCard } from '../SettingsCard';

interface BudgetCardProps {
  /** Undefined when it didn't load (the section waits for it first). */
  usage: AiUsage | undefined;
  /** Picks and Ask by person (#239); null hides it (one person, or no split from the server). */
  people: MemberSpend[] | null;
  onRetryUsage: () => void;
  now: number;
  /** The budget input's text. */
  budget: string;
  onBudget: (text: string) => void;
  /** A 422 about the budget, or a budget that doesn't parse. */
  error: string | null;
}

/** What each person's picks and Ask spent (#239): legend-style, wrapping on one line at 1440 and stacked at 390. */
function SpendByPerson({ people }: { people: MemberSpend[] }) {
  return (
    <div className="set-spend-people">
      <span id="ai-spend-people-h" className="t-micro text-fg-3">
        Picks and Ask by person
      </span>
      <ul className="set-spend-legend" aria-labelledby="ai-spend-people-h">
        {people.map((person) => (
          <li key={person.memberId}>
            <MemberAvatar name={person.name} color={person.color ?? ''} size="xs" />
            {person.name}
            <span className="tabular-nums">{usd(person.cost)}</span>
          </li>
        ))}
      </ul>
    </div>
  );
}

/** This month's spend: the total of the budget, the bar by feature with its legend, by person, and where the pace leads. */
function Spend({ usage, people, now }: { usage: AiUsage; people: MemberSpend[] | null; now: number }) {
  const { spent, of } = spendLine(usage);
  const bar = spendBar(usage);
  const projection = spendProjection(usage, now);
  return (
    <div className="set-spend">
      <div className="set-spend-top">
        <p>
          <span className="big tabular-nums">{spent}</span> <span className="tabular-nums text-fg-2">{of}</span>
        </p>
        <span className="t-small text-fg-3">Resets {budgetMonth(usage.month).resets}</span>
      </div>
      <div className="set-spend-bar" role="img" aria-label={bar.label}>
        {bar.segments.map((segment) =>
          segment.pct > 0 ? <i key={segment.feature} className={`set-spend-${segment.feature}`} style={{ width: `${segment.pct}%` }} /> : null
        )}
      </div>
      <ul className="set-spend-legend" aria-label="Spend by feature">
        {bar.segments.map((segment) => (
          <li key={segment.feature}>
            <i className={`set-spend-${segment.feature}`} aria-hidden="true" />
            {segment.label}
            <span className="tabular-nums">{usd(segment.cost)}</span>
          </li>
        ))}
      </ul>
      {people && <SpendByPerson people={people} />}
      <p className={cx('t-small', projection.warn ? 'text-warning' : 'text-fg-3')}>{projection.text}</p>
    </div>
  );
}

/**
 * Monthly budget (screens/settings/ai-*): what every AI feature spent this
 * server-local month (GET /settings/ai/usage), and the cap they share. An
 * empty budget is no cap. With two or more people, picks and Ask split by
 * person; the classifier is the household's.
 */
export function BudgetCard({ usage, people, onRetryUsage, now, budget, onBudget, error }: BudgetCardProps) {
  return (
    <SettingsCard id="ai-budget-card" title="Monthly budget">
      {usage ? (
        <Spend usage={usage} people={people} now={now} />
      ) : (
        <div className="set-card-note">
          <Banner
            tone="error"
            role="alert"
            title="Couldn’t load this month’s spend"
            action={
              <Button size="sm" onClick={onRetryUsage}>
                <RefreshCw size={15} aria-hidden="true" /> Try again
              </Button>
            }
          >
            The budget below still saves.
          </Banner>
        </div>
      )}

      <SettingRow id="ai-budget" label="Budget" labelFor="ai-budget-input" help="Per calendar month. Leave it empty for no cap." error={error}>
        <div className="set-money">
          <span aria-hidden="true">$</span>
          <input
            id="ai-budget-input"
            className="input"
            type="text"
            inputMode="decimal"
            autoComplete="off"
            placeholder="No cap"
            value={budget}
            aria-invalid={error ? true : undefined}
            aria-describedby={rowDescribedBy('ai-budget', error)}
            onChange={(event) => onBudget(event.target.value)}
          />
        </div>
      </SettingRow>
    </SettingsCard>
  );
}
