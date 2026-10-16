import { CatAvatar } from '@/components/brand';
import { askProblem, type AskProblemContext, turnWhen } from '@/lib/ask/copy';
import { isWorking, type AskTurnView } from '@/lib/ask/turns';
import { AskCard, AskCardSkeleton } from './AskCard';
import { AskProblemCard } from './AskProblemCard';
import { AskSteps, AskTrace } from './AskSteps';
import type { AskLibrary } from './useAskLibrary';

interface AskTurnProps {
  turn: AskTurnView;
  /** The conversation's newest turn: only it offers refine chips and Try again. */
  last: boolean;
  /** Nothing is streaming: chips and Try again can send. */
  idle: boolean;
  library: AskLibrary;
  problemContext: AskProblemContext;
  now: number;
  onRefine: (refinement: string) => void;
  onRetry: (turn: AskTurnView) => void;
  onNew: () => void;
}

function whoLine(turn: AskTurnView, now: number): string {
  if (isWorking(turn)) return turn.live ? (turn.intro ? 'Writing…' : 'Thinking…') : 'Still answering…';
  return turn.createdAt ? turnWhen(turn.createdAt, now) : 'just now';
}

/** Your message, then Noir's answer (ask.js `userMsg`, `noirTurn`): steps while it works, then the trace, the intro, the cards and the refine chips. */
export function AskTurn({ turn, last, idle, library, problemContext, now, onRefine, onRetry, onNew }: AskTurnProps) {
  const working = isWorking(turn);
  const problem = working ? null : askProblem(turn, problemContext);
  // The next card is on its way once the intro starts (the usual order: status…, text…, item…, done)
  const coming = working && turn.live && (turn.intro !== '' || turn.items.length > 0);
  const chips = last && idle && turn.phase === 'done' && turn.refinements.length > 0;

  return (
    <>
      <div className="msg msg--me" data-turn={turn.key}>
        <span className="sr-only">You asked: </span>
        {turn.message}
      </div>
      <article className="msg msg--noir" aria-busy={working || undefined} aria-label="Noir’s answer">
        <div className="msg-who">
          <CatAvatar size={30} />
          <span className="font-semibold">Noir</span>
          <span className="t-small text-fg-3">{whoLine(turn, now)}</span>
        </div>
        <div className="msg-body">
          {working && turn.live && <AskSteps steps={turn.steps} refine={turn.refine} />}
          {!working && turn.steps.length > 0 && <AskTrace steps={turn.steps} elapsedMs={turn.phase === 'done' ? turn.elapsedMs : null} />}
          {turn.intro && <p className="ask-intro">{turn.intro}</p>}
          {(turn.items.length > 0 || coming) && (
            <div className="ask-cards">
              {turn.items.map((item, index) => (
                <AskCard key={`${item.media_type}:${item.tmdb_id}`} item={item} index={index} library={library} isNew={turn.live} />
              ))}
              {coming && <AskCardSkeleton />}
            </div>
          )}
          {problem && (
            <AskProblemCard problem={problem} onRetry={last && idle ? () => onRetry(turn) : undefined} onNew={last ? onNew : undefined} />
          )}
          {turn.phase === 'cancelled' && (
            <p className="msg-note">{turn.items.length > 0 || turn.intro ? 'Stopped. Here’s what Noir had so far.' : 'You stopped this answer.'}</p>
          )}
          {working && !turn.live && <p className="msg-note">Noir is still answering this in another tab or window.</p>}
          {chips && (
            <div className="ask-refine" role="group" aria-label="Refine these picks">
              <span className="t-small text-fg-3" aria-hidden="true">
                Refine
              </span>
              {turn.refinements.map((refinement) => (
                <button key={refinement} type="button" className="fchip" onClick={() => onRefine(refinement)}>
                  {refinement}
                </button>
              ))}
            </div>
          )}
        </div>
      </article>
    </>
  );
}
