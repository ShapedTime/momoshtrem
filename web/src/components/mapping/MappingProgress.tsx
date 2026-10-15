import { Loader, Skeleton, Steps } from '@/components/ui';
import type { AssignmentJob } from '@/lib/api/contract';
import { progressSteps } from '@/lib/mapping/review';

const GHOST = [
  [72, 38],
  [64, 44],
  [80, 30],
  [58, 40],
];

/**
 * While the worker maps the files (HANDOFF §4.3: named steps, never just a
 * spinner): Noir's eyes, the three steps with their details, and the rows to
 * come as ghosts. `done` is the beat before the plan shows.
 */
export function MappingProgress({ job, done }: { job: AssignmentJob; done: boolean }) {
  const steps = progressSteps(job, done);
  return (
    <section className="map-progress page" aria-label="Mapping progress">
      <Loader title={done ? 'Done. Here’s the plan.' : 'Mapping files to episodes'} detail="Usually takes a few seconds" />
      <div className="map-steps" aria-live="polite">
        <Steps steps={steps.map(({ label, detail, state }) => ({ label, detail: detail ?? undefined, state }))} />
      </div>
      <ul className="map-ghost" aria-hidden="true">
        {GHOST.map(([first, second], i) => (
          <li key={i}>
            <Skeleton shape="line" width={`${first}%`} />
            <Skeleton shape="line" width={`${second}%`} />
          </li>
        ))}
      </ul>
    </section>
  );
}
