import { redirect } from 'next/navigation';

/** The legacy torrents page became Activity (#102); old links and bookmarks land there. */
export default function TorrentsPage() {
  redirect('/activity');
}
