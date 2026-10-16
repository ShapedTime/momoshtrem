/**
 * The torrent search sheet (`?sheet=torrent`, #92) is registered, so its deep
 * link opens it, but nothing leads there yet: the Torrents tab and the title
 * pages keep opening the legacy search until the backend's /search/torrents
 * lands. Entry points check this flag, not `getSheet('torrent')`.
 * S10 (#95) deletes it together with those checks.
 */
export const TORRENT_SHEET_LIVE = false;
