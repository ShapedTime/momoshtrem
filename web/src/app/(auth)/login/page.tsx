import type { Metadata } from 'next';
import { redirect } from 'next/navigation';
import { Suspense } from 'react';
import { Mark } from '@/components/brand';
import { getSession, safeRedirect } from '@/lib/auth';
import type { LoginErrorCode } from '@/types/auth';
import { LoginForm, type InitialLoginError } from './LoginForm';
import { PosterWall } from './PosterWall';
import './auth.css';

export const metadata: Metadata = { title: 'Sign in' };

type SearchParams = Promise<Record<string, string | string[] | undefined>>;

const first = (value: string | string[] | undefined) => (Array.isArray(value) ? value[0] : value);

function initialError(code: string | undefined, retry: string | undefined): InitialLoginError | undefined {
  const known: LoginErrorCode[] = ['invalid_credentials', 'missing_credentials', 'rate_limited', 'server_error'];
  if (!code || !known.includes(code as LoginErrorCode)) return undefined;
  const retryAfter = Number.parseInt(retry ?? '', 10);
  return { code: code as LoginErrorCode, retryAfter: Number.isFinite(retryAfter) && retryAfter > 0 ? retryAfter : undefined };
}

/**
 * Server-rendered so the form is in the initial HTML: password managers see
 * a real username + password form and offer to save and fill it. Without
 * JavaScript the form still posts and the server answers with a redirect.
 */
export default async function LoginPage({ searchParams }: { searchParams: SearchParams }) {
  const params = await searchParams;
  const redirectTo = safeRedirect(first(params.redirect));

  const session = await getSession();
  if (session?.authenticated) redirect(redirectTo);

  return (
    <main className="auth">
      <div className="auth-glow" aria-hidden="true" />
      <Suspense fallback={null}>
        <PosterWall />
      </Suspense>
      <div className="auth-scrim" aria-hidden="true" />
      <div className="auth-center">
        <LoginForm redirectTo={redirectTo} initialError={initialError(first(params.error), first(params.retry))} />
        <p className="auth-legal t-small text-fg-3">
          <Mark size={18} />
          <span>momoshtrem · self-hosted</span>
        </p>
      </div>
    </main>
  );
}
