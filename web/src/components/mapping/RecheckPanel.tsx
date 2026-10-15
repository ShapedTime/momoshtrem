'use client';

import { type KeyboardEvent, useEffect, useId, useMemo, useRef, useState } from 'react';
import { CheckCheck, Info, Sparkle, TriangleAlert } from 'lucide-react';
import { CatAvatar, CatEyes } from '@/components/brand';
import { Button, FilterChip, Input, SegmentedControl, Steps } from '@/components/ui';
import { DESKTOP_QUERY } from '@/hooks/useMediaQuery';
import { useRecheck } from '@/hooks/useMappingJob';
import { type AssignmentJobDetail, isJobWorking, type Proposal } from '@/lib/api/contract';
import {
  AI_ERROR_LEAD,
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
  /** What was asked, shown until the job itself says (it's resolving). */
  files: number;
  hint: string;
  phase: 'starting' | 'started' | 'working';
}

/**
 * "Re-check with AI" above the groups (HANDOFF §5.5): a hint, example chips
 * and which files, then Noir works while the groups stay usable, and the
 * rows it changed say Updated. Shown while `job.ai_available`, and kept to
 * explain a 503 when AI went away. Screen readers hear every state through
 * one live region that stays put.
 */
export function RecheckPanel({ detail }: { detail: AssignmentJobDetail }) {
  const { job, proposals } = detail;
  const { start, finished } = useRecheck(job.id);
  const counts = useMemo(() => recheckCounts(proposals), [proposals]);
  const bodyId = useId();
  const panelRef = useRef<HTMLElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const noticeRef = useRef<HTMLParagraphElement>(null);
  const [open, setOpen] = useState(false);
  const [hint, setHint] = useState('');
  const [scope, setScope] = useState<RecheckScope>(() => defaultScope(counts));
  const [run, setRun] = useState<Run | null>(null);
  const [result, setResult] = useState<{ changed: number } | null>(null);
  const [notice, setNotice] = useState<{ kind: 'unavailable' | 'failed'; text: string } | null>(null);

  const working = isJobWorking(job.status);
  /** Count what changed (`updated` was cleared when it started); a failed run sets ai_error instead. */
  const finish = (settled: Proposal[]) => {
    setRun(null);
    setResult({ changed: updatedCount(settled) });
    setOpen(false);
  };
  // A re-check started elsewhere (another tab) shows here too
  if (run === null && working) setRun({ files: job.progress.ai_files ?? 0, hint: job.hint?.text ?? '', phase: 'working' });
  if (run?.phase === 'started' && working) setRun({ ...run, phase: 'working' });
  if (run?.phase === 'working' && !working) finish(proposals);
  useEffect(() => {
    if (result) void finished();
  }, [result, finished]);
  // A 503 or a failed start: the button you pressed may be gone, so focus moves to why
  useEffect(() => {
    if (notice) noticeRef.current?.focus({ preventScroll: true });
  }, [notice]);

  if (!job.ai_available && !notice) return null;
  const unavailable = !job.ai_available;

  const toggle = () => {
    const next = !open;
    setOpen(next);
    setResult(null);
    // Desktop only: on a phone the keyboard would cover the chips. Only if focus hasn't moved on meanwhile.
    if (next && window.matchMedia(DESKTOP_QUERY).matches) {
      const from = document.activeElement;
      window.requestAnimationFrame(() => {
        if (document.activeElement === from || document.activeElement === document.body) inputRef.current?.focus();
      });
    }
  };

  const recheck = async () => {
    const files = counts[scope];
    if (run !== null || files === 0) return;
    const text = hint.trim();
    // The button goes away while Noir works: keep focus on the panel
    if (panelRef.current?.contains(document.activeElement)) panelRef.current.focus({ preventScroll: true });
    setNotice(null);
    setResult(null);
    setRun({ files, hint: text, phase: 'starting' });
    const outcome = await start(text ? { scope, hint: text } : { scope }, files);
    if (outcome.kind === 'started') {
      // Already done by the time it was read again (a quick run): show the result now
      if (outcome.detail && !isJobWorking(outcome.detail.job.status)) finish(outcome.detail.proposals);
      else setRun((current) => current && { ...current, phase: current.phase === 'starting' ? 'started' : current.phase });
      return;
    }
    setRun(null);
    setNotice(
      outcome.kind === 'unavailable'
        ? { kind: 'unavailable', text: AI_UNAVAILABLE }
        : { kind: 'failed', text: `Couldn’t start the re-check. ${outcome.message}` }
    );
  };

  const onHintKey = (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.key !== 'Enter' || event.nativeEvent.isComposing) return;
    event.preventDefault();
    void recheck();
  };

  // While the job works it says what it's doing (its count and hint), whoever started it
  const runFiles = working ? job.progress.ai_files : (run?.files ?? null);
  const runHint = working ? (job.hint?.text ?? '') : (run?.hint ?? '');
  const aiError = run === null ? job.ai_error : null;
  const done = result && !aiError ? doneMessage(result.changed) : null;
  const announcement =
    run ? `${workingTitle(runFiles, runHint !== '')} ${workingDetail(runHint)}`
    : notice ? notice.text
    : aiError ? `${AI_ERROR_LEAD} ${aiError}`
    : result ? (done ? `${done.lead}Updated${done.tail}` : NOTHING_CHANGED)
    : '';

  return (
    <section
      ref={panelRef}
      tabIndex={-1}
      aria-label="Re-check with AI"
      className={cx('map-ai panel', (open || run) && !unavailable && 'is-open', run && 'is-working')}
    >
      <p className="sr-only" role="status" aria-live="polite" aria-atomic="true">
        {announcement}
      </p>
      {run ? (
        <div className="map-ai-working">
          <div className="map-ai-top">
            <span className="map-ai-eyes">
              <CatEyes width={40} />
            </span>
            <div className="map-ai-copy">
              <strong>{workingTitle(runFiles, runHint !== '')}</strong>
              <span className="t-small text-fg-3">{workingDetail(runHint)}</span>
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
              {/* Not next to "Noir can’t take another look" after a 503 */}
              {notice?.kind !== 'unavailable' && <span className="t-small text-fg-2">Noir can take another look. A hint helps with odd numbering.</span>}
            </div>
            {!unavailable && (
              <Button variant="secondary" size="sm" className="map-ai-btn" aria-expanded={open} aria-controls={bodyId} onClick={toggle}>
                <Sparkle size={16} aria-hidden="true" />
                <span>
                  Re-check<span className="map-ai-more"> with AI</span>
                </span>
              </Button>
            )}
          </div>
          {result && !aiError && (
            <p className="map-ai-done">
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
            <p className="map-ai-note is-warning">
              <TriangleAlert size={16} aria-hidden="true" />
              <span>
                <b>{AI_ERROR_LEAD}</b> {aiError}
              </span>
            </p>
          )}
          {notice && (
            <p ref={noticeRef} tabIndex={-1} className={cx('map-ai-note', notice.kind === 'failed' && 'is-warning')}>
              {notice.kind === 'failed' ? <TriangleAlert size={16} aria-hidden="true" /> : <Info size={16} aria-hidden="true" />}
              <span>{notice.text}</span>
            </p>
          )}
          {!unavailable && (
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
                {/* An empty scope has nothing to ask about (the backend settles it at once) */}
                <Button variant="secondary" className="map-ai-go" disabled={counts[scope] === 0} onClick={() => void recheck()}>
                  <Sparkle size={17} aria-hidden="true" />
                  Re-check
                </Button>
              </div>
            </div>
          )}
        </>
      )}
    </section>
  );
}
