import { Media } from '@/components/ui';
import { buildImageUrl } from '@/config/tmdb';
import type { CastMember } from '@/lib/api/contract';

const initials = (name: string) =>
  name
    .split(/\s+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((word) => word[0])
    .join('')
    .toUpperCase();

/** Cast in a row that scrolls inside itself and bleeds to the screen edge. */
export function CastRow({ cast }: { cast: CastMember[] }) {
  if (!cast.length) return null;
  return (
    <section className="section" aria-labelledby="cast-h">
      <div className="section-head">
        <h2 className="t-title-3" id="cast-h">
          Cast
        </h2>
      </div>
      {/* Focusable so the row scrolls from the keyboard too */}
      <ul className="cast-row" aria-labelledby="cast-h" tabIndex={0}>
        {cast.map((member) => {
          const photo = buildImageUrl(member.profile_path, 'profile', 'medium');
          return (
            <li key={`${member.name}-${member.character}`} className="cast">
              <Media shape="square" src={photo} alt="" sizes="72px">
                {!photo && (
                  <span className="initials" aria-hidden="true">
                    {initials(member.name)}
                  </span>
                )}
              </Media>
              <span className="name">{member.name}</span>
              {member.character && <span className="role">{member.character}</span>}
            </li>
          );
        })}
      </ul>
    </section>
  );
}
