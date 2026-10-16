import { redirect } from 'next/navigation';

/** The old movie browser lives on in Discover (#100). */
export default function MoviesPage() {
  redirect('/discover?type=movie');
}
