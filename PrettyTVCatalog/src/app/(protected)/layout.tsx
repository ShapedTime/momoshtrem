import { redirect } from 'next/navigation';
import { getSession } from '@/lib/auth';
import { LOGIN_ROUTE } from '@/config/auth';
import { Header } from '@/components/layout';
import { ToastProvider } from '@/components/ui';

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
    <ToastProvider>
      <div className="min-h-screen bg-bg-primary">
        <Header />
        <main>{children}</main>
      </div>
    </ToastProvider>
  );
}
