import type { ReactNode } from 'react';
import type { TitleFact } from '@/lib/title/facts';

/** The aside column: optional cards on top (On Trakt), then the Details facts. */
export function DetailsAside({ facts, children }: { facts: TitleFact[]; children?: ReactNode }) {
  return (
    <aside className="aside-stack" aria-label="About this title">
      {children}
      {facts.length > 0 && (
        <section className="panel" aria-labelledby="details-h">
          <div className="panel-head">
            <h2 className="t-title-3" id="details-h">
              Details
            </h2>
          </div>
          <dl className="facts-list">
            {facts.map((fact) => (
              <div key={fact.label}>
                <dt>{fact.label}</dt>
                <dd>{fact.value}</dd>
              </div>
            ))}
          </dl>
        </section>
      )}
    </aside>
  );
}
