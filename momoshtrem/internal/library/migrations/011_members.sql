-- Household members (#239, B1 #241; PLAN §5, §6 "Household members"): the
-- people sharing the one login, up to six (member.MaxMembers, checked under
-- an advisory lock by MemberRepository). New table only; 012–014 give the
-- Trakt, picks and Ask tables their member_id. Idempotent, like 003.
--
-- Reversible by deploying the old binary: nothing else reads this table.

-- color is a palette key (member.Colors), checked in Go. A name is unique
-- case-insensitively.
CREATE TABLE IF NOT EXISTS members (
    id         BIGSERIAL PRIMARY KEY,
    name       TEXT NOT NULL CHECK (char_length(name) BETWEEN 1 AND 32),
    color      TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS members_name_key ON members (lower(name));

-- There's always one. The first is whoever has the Trakt connection (its
-- username), else "Me"; trakt_auth has at most one row (CHECK (id = 1)).
-- Its id comes from the sequence (1 in a new table) instead of being set,
-- so the sequence never moves back and a removed member's id is never
-- handed out again: the web keeps the chosen id in a cookie.
INSERT INTO members (name, color)
SELECT COALESCE(NULLIF(btrim(left(btrim(a.username), 32)), ''), 'Me'), 'teal'
FROM (VALUES (1)) one(x) LEFT JOIN trakt_auth a ON TRUE
WHERE NOT EXISTS (SELECT 1 FROM members);
