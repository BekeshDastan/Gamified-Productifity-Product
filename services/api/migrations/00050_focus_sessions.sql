-- +goose Up
CREATE TABLE focus_sessions (
 id uuid NOT NULL, owner_id uuid NOT NULL REFERENCES users(id), request jsonb NOT NULL,
 state text NOT NULL CHECK(state IN('running','paused','awaiting_confirmation','finished','cancelled')),
 mode text NOT NULL CHECK(mode IN('free','pomodoro','quick')), phase text NOT NULL,
 title text NOT NULL, quest_id uuid, goal_id uuid, area_id uuid, timezone text NOT NULL,
 work_minutes integer NOT NULL CHECK(work_minutes BETWEEN 5 AND 120),
 break_minutes integer NOT NULL CHECK(break_minutes BETWEEN 1 AND 60), sound_enabled boolean NOT NULL,
 cycle integer NOT NULL DEFAULT 1 CHECK(cycle>0), version bigint NOT NULL DEFAULT 1 CHECK(version>0),
 started_at timestamptz NOT NULL, updated_at timestamptz NOT NULL, ended_at timestamptz,
 phase_deadline timestamptz, result text NOT NULL DEFAULT '', comment text NOT NULL DEFAULT '',
 PRIMARY KEY(owner_id,id), FOREIGN KEY(owner_id,quest_id) REFERENCES quests(owner_id,id),
 FOREIGN KEY(owner_id,goal_id) REFERENCES goals(owner_id,id), FOREIGN KEY(owner_id,area_id) REFERENCES life_areas(owner_id,id)
);
CREATE UNIQUE INDEX focus_one_current ON focus_sessions(owner_id) WHERE state IN('running','paused','awaiting_confirmation');
CREATE INDEX focus_history_owner ON focus_sessions(owner_id,started_at DESC,id);
CREATE TABLE focus_segments (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY, owner_id uuid NOT NULL, session_id uuid NOT NULL,
 kind text NOT NULL CHECK(kind IN('work','break')), started_at timestamptz NOT NULL, ended_at timestamptz,
 CHECK(ended_at IS NULL OR ended_at>=started_at), FOREIGN KEY(owner_id,session_id) REFERENCES focus_sessions(owner_id,id)
);
CREATE UNIQUE INDEX focus_one_open_segment ON focus_segments(owner_id,session_id) WHERE ended_at IS NULL;
CREATE INDEX focus_segment_session ON focus_segments(owner_id,session_id,id);
CREATE TABLE focus_events (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY, owner_id uuid NOT NULL, session_id uuid NOT NULL,
 kind text NOT NULL, snapshot jsonb NOT NULL, created_at timestamptz NOT NULL,
 FOREIGN KEY(owner_id,session_id) REFERENCES focus_sessions(owner_id,id)
);
CREATE TRIGGER focus_events_immutable BEFORE UPDATE OR DELETE ON focus_events FOR EACH ROW EXECUTE FUNCTION reject_economy_rewrite();
-- +goose Down
DROP TABLE focus_events;
DROP TABLE focus_segments;
DROP TABLE focus_sessions;
