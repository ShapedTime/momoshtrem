import { redirect } from 'next/navigation';
import { getSession } from '@/lib/auth';
import { LOGIN_ROUTE } from '@/config/auth';
import { Header } from '@/components/layout';
import { Providers } from '@/lib/query/Providers';
import { ErrorBoundary } from '@/components/ErrorBoundary';

export default async function ProtectedLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  // Server-side session check (additional layer beyond middleware)
  const session = await getSession();

  if (!session || !session.authenticated) {
    redirect(LOGIN_ROUTE);
  }

  return (
    <Providers>
      <div className="min-h-screen bg-bg-primary">
        <Header />
        <main>
          <ErrorBoundary>{children}</ErrorBoundary>
        </main>
      </div>
    </Providers>
  );
}
