import { Fragment } from 'react';
import { highlight } from '@/lib/title-search/text';

/** `text` with the parts that match the query in `<mark>` (amber text, no fill). */
export function Highlight({ text, query }: { text: string; query: string }) {
  return (
    <>
      {highlight(text, query).map((segment, index) =>
        segment.match ? <mark key={index}>{segment.text}</mark> : <Fragment key={index}>{segment.text}</Fragment>
      )}
    </>
  );
}
