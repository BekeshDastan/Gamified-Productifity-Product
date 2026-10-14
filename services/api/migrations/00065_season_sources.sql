-- +goose Up
CREATE TABLE season_goal_links (
 owner_id uuid NOT NULL,season_id uuid NOT NULL,goal_id uuid NOT NULL,
 is_main boolean NOT NULL DEFAULT false,baseline jsonb NOT NULL,linked_at timestamptz NOT NULL,
 PRIMARY KEY(owner_id,season_id,goal_id),
 FOREIGN KEY(owner_id,season_id) REFERENCES personal_seasons(owner_id,id),
 FOREIGN KEY(owner_id,goal_id) REFERENCES goals(owner_id,id)
);
CREATE UNIQUE INDEX one_main_goal_per_season ON season_goal_links(owner_id,season_id) WHERE is_main;
CREATE TABLE season_challenge_links (
 owner_id uuid NOT NULL,season_id uuid NOT NULL,run_id uuid NOT NULL,
 baseline jsonb NOT NULL,linked_at timestamptz NOT NULL,
 PRIMARY KEY(owner_id,season_id,run_id),
 FOREIGN KEY(owner_id,season_id) REFERENCES personal_seasons(owner_id,id),
 FOREIGN KEY(owner_id,run_id) REFERENCES challenge_runs(owner_id,id)
);
CREATE TABLE season_monthly_boss_links (
 owner_id uuid NOT NULL,season_id uuid NOT NULL,month text NOT NULL,
 timezone text NOT NULL,boss_id uuid NOT NULL,source_version bigint NOT NULL,
 linked_at timestamptz NOT NULL,
 PRIMARY KEY(owner_id,season_id,month),
 FOREIGN KEY(owner_id,season_id) REFERENCES personal_seasons(owner_id,id),
 FOREIGN KEY(owner_id,boss_id) REFERENCES bosses(owner_id,id)
);
-- +goose Down
DROP TABLE season_monthly_boss_links;
DROP TABLE season_challenge_links;
DROP TABLE season_goal_links;
