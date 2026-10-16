import { redirect } from 'next/navigation';

/** The old show browser lives on in Discover (#100). */
export default function TVShowsPage() {
  redirect('/discover?type=tv');
}
