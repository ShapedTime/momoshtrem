'use client';

import { useCallback, useEffect, useRef, useState } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import { Check, ChevronDown, Copy, Pause, RefreshCw, Tv, X } from 'lucide-react';
import { CatEyes, Lockup } from '@/components/brand';
import { InfuseSetupSheet } from '@/components/shell/InfuseSetupSheet';
import { Button, copyText, Sheet, useToast } from '@/components/ui';
import { apiFetch, parseErrorBody } from '@/lib/api/client';
import type { BackendUnreachableBody } from '@/lib/api/contract';
import { safeRedirect } from '@/lib/auth/redirect';

const FIRST_RETRY_S = 15;
const LATER_RETRY_S = 30;

interface CheckResult {
  at: Date;
  httpStatus: number | null;
  code: string;
  message: string;
  webdavReachable: boolean | null;
}

/** Ask the web app's /api/status whether momoshtrem answers again. */
async function checkBackend(): Promise<CheckResult | 'ok'> {
  const at = new Date();
  try {
    const response = await apiFetch('/api/status', { cache: 'no-store' });
    if (response.ok) return 'ok';
    const body = (await response.json().catch(() => undefined)) as Partial<BackendUnreachableBody> | undefined;
    const { code, message } = parseErrorBody(body);
    return {
      at,
      httpStatus: response.status,
      code: code ?? 'error',
      message: message ?? response.statusText,
      webdavReachable: body?.webdav?.reachable ?? null,
    };
  } catch (error) {
    return { at, httpStatus: null, code: 'network_error', message: error instanceof Error ? error.message : 'Network error', webdavReachable: null };
  }
}

function WorksRow({ ok, paused, title, detail }: { ok?: boolean; paused?: boolean; title: string; detail: string }) {
  const tone = paused ? 'border-error-line bg-error-bg text-error' : ok ? 'border-ready-line bg-ready-bg text-ready' : 'border-warning-line bg-warning-bg text-warning';
  const Icon = paused ? Pause : ok ? Check : X;
  return (
    <li className="list-row items-start py-3">
      <span className={`mt-0.5 grid size-7 shrink-0 place-items-center rounded-full border ${tone}`} aria-hidden="true">
        <Icon size={15} strokeWidth={2.4} />
      </span>
      <div className="grow">
        <span className="font-semibold text-fg-1">{title}</span>
        <span className="t-small text-fg-3">{detail}</span>
      </div>
    </li>
  );
}

/**
 * Full-page "momoshtrem isn't responding" (screens/system/offline). Retries on
 * its own (15 s, then every 30 s, skipped while the tab is hidden) and goes
 * back to where you were as soon as the server answers.
 */
export function OfflineView() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const destination = safeRedirect(searchParams.get('from'));
  const { toast } = useToast();
  const [result, setResult] = useState<CheckResult | null>(null);
  const [checking, setChecking] = useState(true);
  const [attempt, setAttempt] = useState(1);
  const [nextAt, setNextAt] = useState<number | null>(null);
  const [now, setNow] = useState(() => Date.now());
  const [howToOpen, setHowToOpen] = useState(false);
  const busy = useRef(false);
  const mounted = useRef(true);

  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);

  const settle = useCallback(
    (outcome: CheckResult | 'ok', attemptNumber: number) => {
      // A check that finishes after you've left mustn't pull you back
      if (!mounted.current) return;
      if (outcome === 'ok') {
        router.replace(destination);
        return;
      }
      setResult(outcome);
      setChecking(false);
      setAttempt(attemptNumber + 1);
      const wait = attemptNumber === 1 ? FIRST_RETRY_S : LATER_RETRY_S;
      setNow(Date.now());
      setNextAt(Date.now() + wait * 1000);
    },
    [destination, router]
  );

  const runCheck = useCallback(async () => {
    if (busy.current) return;
    busy.current = true;
    setChecking(true);
    const outcome = await checkBackend();
    busy.current = false;
    settle(outcome, attempt);
  }, [attempt, settle]);

  // First check on arrival
  useEffect(() => {
    let cancelled = false;
    busy.current = true;
    checkBackend().then((outcome) => {
      busy.current = false;
      if (!cancelled) settle(outcome, 1);
    });
    return () => {
      cancelled = true;
    };
  }, [settle]);

  // Tick the countdown and check again when it runs out (skipped while the tab is hidden)
  useEffect(() => {
    if (checking || nextAt === null) return;
    const timer = window.setInterval(() => {
      if (document.hidden) return;
      const current = Date.now();
      setNow(current);
      if (current >= nextAt) void runCheck();
    }, 1000);
    return () => window.clearInterval(timer);
  }, [checking, nextAt, runCheck]);

  const secondsLeft = nextAt === null ? FIRST_RETRY_S : Math.max(0, Math.ceil((nextAt - now) / 1000));

  const webdav = result?.webdavReachable;
  const details = result
    ? [
        `GET /api/status → ${result.httpStatus ?? 'no answer'}`,
        `code: ${result.code}`,
        `message: ${result.message}`,
        `webdav: ${webdav === null ? 'unknown' : webdav ? 'answering' : 'not answering'}`,
        `checked: ${result.at.toISOString()}`,
      ].join('\n')
    : '';

  return (
    <div className="page flex min-h-dvh flex-col pb-10" style={{ paddingTop: 'calc(env(safe-area-inset-top, 0px) + 24px)' }}>
      <div className="mx-auto flex w-full max-w-[1100px] items-center justify-between">
        <Lockup height={26} />
        <Button variant="ghost" size="sm" onClick={() => setHowToOpen(true)}>
          <Tv size={16} aria-hidden="true" /> Infuse setup
        </Button>
      </div>

      <div className="mx-auto mt-12 flex w-full max-w-[560px] flex-col items-center gap-6 text-center sm:mt-20">
        <div className="grid h-24 w-[168px] place-items-center rounded-[50%] bg-[radial-gradient(closest-side,rgb(242_179_61/0.06),transparent)] opacity-60">
          <CatEyes width={96} still />
        </div>
        <div className="flex flex-col gap-3">
          <h1 className="text-[clamp(1.75rem,4vw,2.5rem)] leading-[1.1] font-semibold tracking-[-0.03em] text-balance">
            momoshtrem isn’t responding
          </h1>
          <p className="t-body-l text-fg-2">
            The server isn’t answering right now. Nothing is lost: your library, mappings and settings live on the server
            and will be right here when it’s back.
          </p>
        </div>

        <ul className="panel w-full text-left" aria-label="What still works">
          {webdav === true ? (
            <WorksRow ok title="Infuse keeps playing ready titles" detail="WebDAV still answers. Only the app’s API is quiet." />
          ) : webdav === false ? (
            <WorksRow title="Infuse can’t play right now either" detail="WebDAV isn’t answering, so the whole server is probably down or restarting." />
          ) : (
            <WorksRow ok title="Infuse may still play ready titles" detail="It talks to WebDAV directly, not to this page." />
          )}
          <WorksRow ok title="Downloads pick up where they stopped" detail="No need to re-add anything when it’s back." />
          <WorksRow paused title="Search, adding and Activity wait" detail="They need the server. This page checks again on its own." />
        </ul>

        <div className="flex flex-col items-center gap-2">
          <Button variant="primary" size="lg" className="min-w-[200px]" loading={checking} onClick={() => void runCheck()}>
            <RefreshCw size={18} aria-hidden="true" /> Retry now
          </Button>
          <p className="t-small text-fg-3">{checking ? 'Checking…' : `Trying again in ${secondsLeft} s · attempt ${attempt}`}</p>
          {/* Announce each check and its outcome, not every tick of the countdown */}
          <p className="sr-only" role="status" aria-live="polite">
            {checking ? 'Checking…' : 'Still not answering. This page keeps trying on its own.'}
          </p>
        </div>

        {result && (
          <details className="w-full text-left">
            <summary className="mx-auto flex w-fit cursor-pointer list-none items-center gap-1.5 rounded-sm text-small text-fg-3 hover:text-fg-2">
              Technical details <ChevronDown size={16} aria-hidden="true" />
            </summary>
            <div className="mt-3 flex flex-col gap-2">
              <pre className="t-mono overflow-x-auto rounded-md border border-line-1 bg-ink-0 p-3 text-fg-2">{details}</pre>
              <Button
                variant="ghost"
                size="sm"
                className="self-start"
                onClick={async () => toast((await copyText(details)) ? { tone: 'ready', icon: Copy, title: 'Copied' } : { tone: 'error', title: 'Couldn’t copy' })}
              >
                <Copy size={15} aria-hidden="true" /> Copy details
              </Button>
            </div>
          </details>
        )}
      </div>

      <Sheet open={howToOpen} onOpenChange={setHowToOpen} size="narrow" title="Play it in Infuse">
        <InfuseSetupSheet />
      </Sheet>
    </div>
  );
}
