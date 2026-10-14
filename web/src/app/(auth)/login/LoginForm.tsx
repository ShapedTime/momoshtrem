'use client';

import { type FormEvent, useEffect, useId, useRef, useState, useSyncExternalStore } from 'react';
import { CircleAlert, Clock, Eye, EyeOff } from 'lucide-react';
import { CAT_EYE_LEFT, CAT_EYE_RIGHT, CAT_HEAD, CAT_PUPIL_LEFT, CAT_PUPIL_RIGHT } from '@/components/brand/geometry';
import { Banner, Button, IconButton, Input } from '@/components/ui';
import type { LoginErrorCode, LoginResponse } from '@/types/auth';

export interface InitialLoginError {
  code: LoginErrorCode;
  retryAfter?: number;
}

type Feedback =
  | { kind: 'none' }
  | { kind: 'invalid'; remaining?: number }
  | { kind: 'missing' }
  | { kind: 'ratelimited' }
  | { kind: 'server' };

function feedbackFor(code: LoginErrorCode, remaining?: number): Feedback {
  switch (code) {
    case 'invalid_credentials':
      return { kind: 'invalid', remaining };
    case 'missing_credentials':
      return { kind: 'missing' };
    case 'rate_limited':
      return { kind: 'ratelimited' };
    default:
      return { kind: 'server' };
  }
}

const formatCountdown = (seconds: number) => `${Math.floor(seconds / 60)}:${String(seconds % 60).padStart(2, '0')}`;

function greetingFor(hour: number): string {
  if (hour >= 5 && hour < 12) return 'Morning.';
  if (hour >= 12 && hour < 18) return 'Afternoon.';
  return 'Evening.';
}

/** Local-time greeting; the server can't know the visitor's time zone. */
function useGreeting(): string {
  return useSyncExternalStore(
    () => () => {},
    () => greetingFor(new Date().getHours()),
    () => 'Welcome back.'
  );
}

/** Noir: dark head, amber eyes whose pupils follow the username and close for the password. */
function NoirWatch({ look }: { look: number }) {
  const backlight = useId();
  const pupil = ([cx, cy]: readonly [number, number]) => <ellipse className="pupil" cx={cx} cy={cy} rx="1.25" ry="3.1" fill="#07080b" />;
  return (
    <svg className="noir" width="92" height="92" viewBox="0 0 64 64" aria-hidden="true" focusable="false" style={{ ['--look' as string]: `${look}px` }}>
      <defs>
        <radialGradient id={backlight} cx="0.5" cy="0.62" r="0.5">
          <stop offset="0" stopColor="#f2b33d" stopOpacity="0.28" />
          <stop offset="0.55" stopColor="#c9772a" stopOpacity="0.08" />
          <stop offset="1" stopColor="#c9772a" stopOpacity="0" />
        </radialGradient>
      </defs>
      <circle cx="32" cy="38" r="32" fill={`url(#${backlight})`} />
      <path d={CAT_HEAD} fill="#07080b" stroke="#ffd27a" strokeOpacity="0.32" strokeWidth="0.6" />
      <g className="noir-eye">
        <g className="noir-blink">
          <path d={CAT_EYE_LEFT} fill="#ffc95e" />
          {pupil(CAT_PUPIL_LEFT)}
        </g>
      </g>
      <g className="noir-eye">
        <g className="noir-blink">
          <path d={CAT_EYE_RIGHT} fill="#ffc95e" />
          {pupil(CAT_PUPIL_RIGHT)}
        </g>
      </g>
      <g className="noir-lids" fill="none" stroke="#f2b33d" strokeWidth="1.3" strokeLinecap="round">
        <path d="M15.5 37.6q6.4 3.6 12.4 0" />
        <path d="M36.1 37.6q6.4 3.6 12.4 0" />
      </g>
    </svg>
  );
}

interface LoginFormProps {
  /** Validated destination after signing in. */
  redirectTo: string;
  /** Error from a no-JS round trip (?error=…). */
  initialError?: InitialLoginError;
}

/**
 * A real form (method=post, action=/api/auth) with username + current-password
 * fields, so password managers save and fill it. With JavaScript it submits
 * with fetch and then does a full navigation, which password managers treat
 * as a successful sign-in. Inputs are never disabled.
 */
export function LoginForm({ redirectTo, initialError }: LoginFormProps) {
  const greeting = useGreeting();
  const [feedback, setFeedback] = useState<Feedback>(() => (initialError ? feedbackFor(initialError.code) : { kind: 'none' }));
  const [secondsLeft, setSecondsLeft] = useState(initialError?.retryAfter ?? 60);
  const [submitting, setSubmitting] = useState(false);
  const [showPassword, setShowPassword] = useState(false);
  const [passwordFocused, setPasswordFocused] = useState(false);
  const [look, setLook] = useState(0);
  const usernameRef = useRef<HTMLInputElement>(null);
  const passwordRef = useRef<HTMLInputElement>(null);
  const retryAfterRef = useRef(initialError?.retryAfter ?? 60);

  const rateLimited = feedback.kind === 'ratelimited';
  const invalid = feedback.kind === 'invalid';

  // Count down the pause; the button comes back when it ends
  useEffect(() => {
    if (!rateLimited) return;
    const endsAt = Date.now() + retryAfterRef.current * 1000;
    const timer = window.setInterval(() => {
      const left = Math.max(0, Math.ceil((endsAt - Date.now()) / 1000));
      setSecondsLeft(left);
      if (left === 0) setFeedback({ kind: 'none' });
    }, 1000);
    return () => window.clearInterval(timer);
  }, [rateLimited]);

  function clearFieldErrors() {
    if (feedback.kind === 'invalid' || feedback.kind === 'missing') setFeedback({ kind: 'none' });
  }

  async function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (submitting || rateLimited) return;

    const data = new FormData(event.currentTarget);
    const username = String(data.get('username') ?? '');
    const password = String(data.get('password') ?? '');
    if (!username.trim() || !password) {
      setFeedback({ kind: 'missing' });
      (username.trim() ? passwordRef : usernameRef).current?.focus();
      return;
    }

    setSubmitting(true);
    try {
      const response = await fetch('/api/auth', {
        method: 'POST',
        headers: { Accept: 'application/json' },
        body: data,
        credentials: 'same-origin',
      });
      const body = (await response.json().catch(() => null)) as LoginResponse | null;
      if (body?.ok) {
        // Full navigation (not a client transition): password managers treat it as success
        window.location.assign(body.redirect);
        return;
      }
      if (body && !body.ok) {
        if (body.error.code === 'rate_limited') {
          retryAfterRef.current = body.retry_after ?? 60;
          setSecondsLeft(retryAfterRef.current);
        }
        setFeedback(feedbackFor(body.error.code, body.remaining_attempts));
        if (body.error.code === 'invalid_credentials') {
          passwordRef.current?.focus();
          passwordRef.current?.select();
        }
      } else {
        setFeedback({ kind: 'server' });
      }
    } catch {
      setFeedback({ kind: 'server' });
    }
    setSubmitting(false);
  }

  const eyes = rateLimited ? 'sleepy' : passwordFocused ? 'closed' : 'open';
  const caption = passwordFocused ? (showPassword ? 'Still not looking.' : 'Noir won’t look.') : '';
  const describedBy = invalid ? 'auth-msg' : feedback.kind === 'missing' ? 'auth-empty' : undefined;
  const remaining = feedback.kind === 'invalid' ? feedback.remaining : undefined;

  return (
    <section className="auth-card" aria-labelledby="auth-h" data-eyes={eyes}>
      <div className="auth-cat">
        <NoirWatch look={look} />
        <p className="auth-caption t-small" aria-live="polite">
          {caption}
        </p>
      </div>

      <div className="auth-titles">
        <h1 className="t-title-1" id="auth-h">
          {greeting}
        </h1>
        <p className="text-fg-2">
          Sign in to <span className="font-semibold text-fg-1">momoshtrem</span> to pick tonight’s watch.
        </p>
      </div>

      {invalid && (
        <Banner tone="error" role="alert" id="auth-msg" title="Wrong username or password" className="auth-msg">
          {remaining !== undefined && remaining > 0
            ? `Check that caps lock is off. You have ${remaining} more ${remaining === 1 ? 'try' : 'tries'} before a short pause.`
            : 'Check that caps lock is off.'}
        </Banner>
      )}
      {rateLimited && (
        <Banner role="status" id="auth-msg" icon={Clock} title="Too many attempts" className="auth-msg auth-msg--calm">
          Try again in <b className="t-num">{formatCountdown(secondsLeft)}</b>. This keeps guessers out. Your account and
          library are fine.
        </Banner>
      )}
      {feedback.kind === 'server' && (
        <Banner tone="error" role="alert" id="auth-msg" title="Couldn’t sign in" className="auth-msg">
          The server didn’t answer. Try again in a moment.
        </Banner>
      )}

      <form className="auth-form" method="post" action="/api/auth" noValidate onSubmit={onSubmit}>
        <input type="hidden" name="redirect" value={redirectTo} />
        <Input
          ref={usernameRef}
          label="Username"
          name="username"
          type="text"
          size="lg"
          autoComplete="username"
          autoCapitalize="none"
          autoCorrect="off"
          spellCheck={false}
          enterKeyHint="next"
          required
          aria-invalid={invalid || undefined}
          aria-describedby={describedBy}
          onFocus={(event) => setLook(Math.max(-1.6, Math.min(1.6, (event.currentTarget.value.length - 8) * 0.22)))}
          onChange={(event) => {
            clearFieldErrors();
            setLook(Math.max(-1.6, Math.min(1.6, (event.currentTarget.value.length - 8) * 0.22)));
          }}
          onBlur={() => setLook(0)}
        />
        <Input
          ref={passwordRef}
          label="Password"
          name="password"
          type={showPassword ? 'text' : 'password'}
          size="lg"
          autoComplete="current-password"
          autoCapitalize="none"
          autoCorrect="off"
          spellCheck={false}
          enterKeyHint="go"
          required
          aria-invalid={invalid || undefined}
          aria-describedby={describedBy}
          onFocus={() => setPasswordFocused(true)}
          onBlur={() => setPasswordFocused(false)}
          onChange={clearFieldErrors}
          end={
            <IconButton
              className="auth-reveal"
              aria-label={showPassword ? 'Hide password' : 'Show password'}
              aria-pressed={showPassword}
              onClick={() => setShowPassword((shown) => !shown)}
            >
              {showPassword ? <EyeOff size={20} aria-hidden="true" /> : <Eye size={20} aria-hidden="true" />}
            </IconButton>
          }
        />
        {feedback.kind === 'missing' && (
          <p className="error-text" id="auth-empty">
            <CircleAlert size={15} aria-hidden="true" className="mt-0.5 shrink-0" />
            <span>Enter your username and password.</span>
          </p>
        )}
        <Button type="submit" variant="primary" size="lg" block loading={submitting} disabled={rateLimited} className="auth-submit">
          {rateLimited ? `Try again in ${formatCountdown(secondsLeft)}` : 'Sign in'}
        </Button>
        {submitting && (
          <p className="sr-only" role="status">
            Signing in…
          </p>
        )}
      </form>

      <p className="auth-foot t-small text-fg-3">
        Forgot it? It’s <code>APP_PASSWORD</code> in the server’s <code>.env</code>.
      </p>
    </section>
  );
}
