import { redirect } from 'next/navigation';
import { getSession } from '@/lib/auth';
import { LOGIN_ROUTE } from '@/config/auth';

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
    <div className="min-h-screen bg-neutral-950">
      {/* Header will be added in Task 5 */}
      <main>{children}</main>
    </div>
  );
}
