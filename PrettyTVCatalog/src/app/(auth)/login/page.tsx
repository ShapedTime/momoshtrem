'use client';

import { useState, FormEvent, Suspense } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import type { LoginResponse } from '@/types/auth';
import { DEFAULT_REDIRECT } from '@/config/auth';

function LoginForm() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const [password, setPassword] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(false);

  const redirectTo = searchParams.get('redirect') || DEFAULT_REDIRECT;

  async function handleSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setError(null);
    setIsLoading(true);

    try {
      const response = await fetch('/api/auth', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({ password }),
      });

      const data: LoginResponse = await response.json();

      if (data.success) {
        router.push(redirectTo);
        router.refresh();
      } else {
        setError(data.error || 'Login failed');
        setPassword('');
      }
    } catch {
      setError('An error occurred. Please try again.');
    } finally {
      setIsLoading(false);
    }
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-6">
      {/* Error Message */}
      {error && (
        <div
          className="bg-red-900/20 border border-red-500/30 text-red-400
                     rounded-md px-4 py-3 text-sm"
          role="alert"
        >
          {error}
        </div>
      )}

      {/* Password Input */}
      <div>
        <label
          htmlFor="password"
          className="block text-sm font-medium text-neutral-400 mb-2"
        >
          Password
        </label>
        <input
          id="password"
          name="password"
          type="password"
          autoComplete="current-password"
          required
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          disabled={isLoading}
          className="w-full h-12 px-4
                   bg-neutral-800 border border-neutral-700 rounded-md
                   text-white placeholder-neutral-500
                   focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent
                   disabled:opacity-50 disabled:cursor-not-allowed
                   transition-colors duration-200"
          placeholder="Enter password"
        />
      </div>

      {/* Submit Button */}
      <button
        type="submit"
        disabled={isLoading || !password}
        className="w-full h-12
                 bg-red-600 hover:bg-red-700
                 text-white font-semibold rounded-md
                 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 focus:ring-offset-neutral-900
                 disabled:opacity-50 disabled:cursor-not-allowed
                 transition-colors duration-200"
      >
        {isLoading ? (
          <span className="flex items-center justify-center gap-2">
            <svg
              className="animate-spin h-5 w-5"
              xmlns="http://www.w3.org/2000/svg"
              fill="none"
              viewBox="0 0 24 24"
            >
              <circle
                className="opacity-25"
                cx="12"
                cy="12"
                r="10"
                stroke="currentColor"
                strokeWidth="4"
              />
              <path
                className="opacity-75"
                fill="currentColor"
                d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
              />
            </svg>
            Signing in...
          </span>
        ) : (
          'Sign In'
        )}
      </button>
    </form>
  );
}

export default function LoginPage() {
  return (
    <main className="min-h-screen flex items-center justify-center bg-black px-4">
      <div className="w-full max-w-md">
        {/* Logo/Title */}
        <div className="text-center mb-8">
          <h1 className="text-3xl sm:text-4xl font-bold text-white mb-2">
            PrettyTVCatalog
          </h1>
          <p className="text-neutral-400">Enter your password to continue</p>
        </div>

        {/* Login Card */}
        <div className="bg-neutral-900 rounded-lg p-6 sm:p-8 shadow-2xl border border-neutral-800">
          <Suspense
            fallback={
              <div className="flex justify-center py-8">
                <div className="animate-spin h-8 w-8 border-2 border-white border-t-transparent rounded-full" />
              </div>
            }
          >
            <LoginForm />
          </Suspense>
        </div>

        {/* Footer */}
        <p className="text-center text-neutral-600 text-sm mt-6">
          Streaming powered by distribyted
        </p>
      </div>
    </main>
  );
}
