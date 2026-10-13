-- +goose Up
CREATE TABLE habit_archives (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,owner_id uuid NOT NULL,habit_id uuid NOT NULL,
 archived_at timestamptz NOT NULL,restored_at timestamptz,archive_reason text NOT NULL,restore_reason text,
 CHECK(restored_at IS NULL OR restored_at>=archived_at),FOREIGN KEY(owner_id,habit_id) REFERENCES habits(owner_id,id)
);
CREATE UNIQUE INDEX habit_archives_open ON habit_archives(owner_id,habit_id) WHERE restored_at IS NULL;
-- +goose Down
DROP TABLE habit_archives;
