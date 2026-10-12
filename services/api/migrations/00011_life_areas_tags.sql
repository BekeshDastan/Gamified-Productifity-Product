-- +goose Up
CREATE TABLE life_areas (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 60),
    color text NOT NULL CHECK (color ~ '^#[0-9a-f]{6}$'),
    icon text NOT NULL CHECK (icon IN ('book','briefcase','code','languages','activity','rocket','heart','sprout','tag')),
    archived boolean NOT NULL DEFAULT false,
    version bigint NOT NULL DEFAULT 1 CHECK (version>0),
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    UNIQUE(owner_id,id)
);
CREATE UNIQUE INDEX life_areas_name ON life_areas(owner_id,lower(name));
CREATE INDEX life_areas_owner ON life_areas(owner_id,archived);
CREATE TABLE tags (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 60),
    color text NOT NULL CHECK (color ~ '^#[0-9a-f]{6}$'),
    icon text NOT NULL CHECK (icon IN ('book','briefcase','code','languages','activity','rocket','heart','sprout','tag')),
    archived boolean NOT NULL DEFAULT false,
    version bigint NOT NULL DEFAULT 1 CHECK (version>0),
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    UNIQUE(owner_id,id)
);
CREATE UNIQUE INDEX tags_name ON tags(owner_id,lower(name));
CREATE INDEX tags_owner ON tags(owner_id,archived);
-- +goose Down
DROP TABLE tags;
DROP TABLE life_areas;
