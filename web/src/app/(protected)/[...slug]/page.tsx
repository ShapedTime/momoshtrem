import { notFound } from 'next/navigation';

/** Unknown paths 404 inside the shell (so the tab bar and top bar stay). */
export default function UnknownPage(): never {
  notFound();
}
