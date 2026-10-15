'use client';

import { type KeyboardEvent, useEffect, useId, useMemo, useRef, useState } from 'react';
import { CheckCheck, Info, Sparkle, TriangleAlert } from 'lucide-react';
import { CatAvatar, CatEyes } from '@/components/brand';
import { Button, FilterChip, Input, SegmentedControl, Steps } from '@/components/ui';
import { DESKTOP_QUERY } from '@/hooks/useMediaQuery';
import { useRecheck } from '@/hooks/useMappingJob';
import { type AssignmentJobDetail, isJobWorking, type Proposal } from '@/lib/api/contract';
import {
  AI_UNAVAILABLE,
  defaultScope,
  doneMessage,
  HINT_MAX,
  NOTHING_CHANGED,
  recheckCounts,
  type RecheckScope,
  updatedCount,
  WORKING_STEPS,
  workingDetail,
  workingTitle,
} from '@/lib/mapping/recheck';
import { cx } from '@/lib/utils/cx';

/**
 * A re-check from Re-check until the job settles: `starting` while the POST
 * is on its way, `started` once the server took it (202, or one was already
 * running), `working` once the page's job says so. It's over when the job
 * stops working after that: the page's copy of the job can lag the cache by
 * a render, so "not working" before it was seen working means nothing.
 */
interface Run {
  files: number | null;
  hint: string;
  phase: 'starting' | 'started' | 'working';
}

/**
 * "Re-check with AI" above the groups (HANDOFF §5.5): a hint, example chips
 * and which files, then Noir works while the groups stay usable, and the
 * rows it changed say Updated. Only rendered while `job.ai_available`.
 */
export function RecheckPanel({ detail }: { detail: AssignmentJobDetail }) {
  const { job, proposals } = detail;
  const { start, starting, finished } = useRecheck(job.id);
  const counts = useMemo(() => recheckCounts(proposals), [proposals]);
  const bodyId = useId();
  const panelRef = useRef<HTMLElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const [open, setOpen] = useState(false);
  const [hint, setHint] = useState('');
  const [scope, setScope] = useState<RecheckScope>(() => defaultScope(counts));
  const [run, setRun] = useState<Run | null>(null);
  const [result, setResult] = useState<{ changed: number } | null>(null);
  const [notice, setNotice] = useState<{ tone: 'calm' | 'warning'; text: string } | null>(null);

  const working = isJobWorking(job.status);
  /** Count what changed (`updated` was cleared when it started); a failed run sets ai_error instead. */
  const finish = (settled: Proposal[]) => {
    setRun(null);
    setResult({ changed: updatedCount(settled) });
    setOpen(false);
  };
  // A re-check started elsewhere (another tab) shows here too
  if (run === null && working) setRun({ files: job.progress.ai_files, hint: job.hint?.text ?? '', phase: 'working' });
  if (run?.phase === 'started' && working) setRun({ ...run, phase: 'working' });
  if (run?.phase === 'working' && !working) finish(proposals);
  useEffect(() => {
    if (result) void finished();
  }, [result, finished]);

  const toggle = () => {
    const next = !open;
    setOpen(next);
    setResult(null);
    // Desktop only: on a phone the keyboard would cover the chips
    if (next && window.matchMedia(DESKTOP_QUERY).matches) window.requestAnimationFrame(() => inputRef.current?.focus());
  };

  const recheck = async () => {
    if (starting || run !== null || counts[scope] === 0) return;
    const text = hint.trim();
    // The button goes away while Noir works: keep focus on the panel
    if (panelRef.current?.contains(document.activeElement)) panelRef.current.focus({ preventScroll: true });
    setNotice(null);
    setResult(null);
    setRun({ files: counts[scope], hint: text, phase: 'starting' });
    const outcome = await start(text ? { scope, hint: text } : { scope });
    if (outcome.kind === 'started') {
      // Already done by the time it was read again (a quick run): show the result now
      if (outcome.detail && !isJobWorking(outcome.detail.job.status)) finish(outcome.detail.proposals);
      else setRun((current) => current && { ...current, phase: current.phase === 'starting' ? 'started' : current.phase });
      return;
    }
    setRun(null);
    setNotice(
      outcome.kind === 'unavailable'
        ? { tone: 'calm', text: AI_UNAVAILABLE }
        : { tone: 'warning', text: `Couldn’t start the re-check. ${outcome.message}` }
    );
  };

  const onHintKey = (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.key !== 'Enter' || event.nativeEvent.isComposing) return;
    event.preventDefault();
    void recheck();
  };

  const aiError = run === null ? job.ai_error : null;
  const done = result && !aiError ? doneMessage(result.changed) : null;

  return (
    <section
      ref={panelRef}
      tabIndex={-1}
      aria-label="Re-check with AI"
      className={cx('map-ai panel', (open || run) && 'is-open', run && 'is-working')}
    >
      {run ? (
        <div className="map-ai-working" role="status">
          <div className="map-ai-top">
            <span className="map-ai-eyes">
              <CatEyes width={40} />
            </span>
            <div className="map-ai-copy">
              <strong>{workingTitle(run.files ?? job.progress.ai_files, run.hint !== '')}</strong>
              <span className="t-small text-fg-3">{workingDetail(run.hint)}</span>
            </div>
          </div>
          <Steps inline steps={[...WORKING_STEPS]} />
        </div>
      ) : (
        <>
          <div className="map-ai-top">
            <CatAvatar size={36} className="map-ai-avatar" />
            <div className="map-ai-copy">
              <strong>Not quite right?</strong>
              <span className="t-small text-fg-2">Noir can take another look. A hint helps with odd numbering.</span>
            </div>
            <Button variant="secondary" size="sm" className="map-ai-btn" aria-expanded={open} aria-controls={bodyId} onClick={toggle}>
              <Sparkle size={16} aria-hidden="true" />
              <span>
                Re-check<span className="map-ai-more"> with AI</span>
              </span>
            </Button>
          </div>
          {result && !aiError && (
            <p className="map-ai-done" role="status">
              <CheckCheck size={16} aria-hidden="true" />
              <span>
                {done ? (
                  <>
                    {done.lead}
                    <b>Updated</b>
                    {done.tail}
                  </>
                ) : (
                  NOTHING_CHANGED
                )}
              </span>
            </p>
          )}
          {aiError && (
            <p className="map-ai-note is-warning" role="status">
              <TriangleAlert size={16} aria-hidden="true" />
              <span>
                <b>The re-check didn’t finish.</b> Your suggestions are unchanged. {aiError}
              </span>
            </p>
          )}
          <div className="map-ai-body" id={bodyId} hidden={!open}>
            <Input
              ref={inputRef}
              label="Hint for Noir (optional)"
              placeholder="e.g. these are absolute numbers"
              value={hint}
              onChange={(event) => setHint(event.target.value)}
              onKeyDown={onHintKey}
              autoComplete="off"
              enterKeyHint="go"
              maxLength={HINT_MAX}
            />
            {job.suggested_hints.length > 0 && (
              <div className="fchips" role="group" aria-label="Example hints">
                {job.suggested_hints.map((suggestion) => (
                  <FilterChip key={suggestion} pressed={hint.trim() === suggestion} onPressedChange={(pressed) => setHint(pressed ? suggestion : '')}>
                    {suggestion}
                  </FilterChip>
                ))}
              </div>
            )}
            <div className="map-ai-run">
              <SegmentedControl
                aria-label="Which files to re-check"
                value={scope}
                onChange={setScope}
                options={[
                  { value: 'unresolved', label: `Unresolved · ${counts.unresolved}` },
                  { value: 'all', label: `All files · ${counts.all}` },
                ]}
              />
              <Button variant="secondary" className="map-ai-go" loading={starting} disabled={counts[scope] === 0} onClick={() => void recheck()}>
                <Sparkle size={17} aria-hidden="true" />
                Re-check
              </Button>
            </div>
            {notice && (
              <p className={cx('map-ai-note', notice.tone === 'warning' && 'is-warning')} role="status">
                {notice.tone === 'warning' ? <TriangleAlert size={16} aria-hidden="true" /> : <Info size={16} aria-hidden="true" />}
                <span>{notice.text}</span>
              </p>
            )}
          </div>
        </>
      )}
    </section>
  );
}
