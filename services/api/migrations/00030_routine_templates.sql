-- +goose Up
CREATE TABLE routine_templates (
 owner_id uuid NOT NULL REFERENCES users(id),
 id text NOT NULL,
 payload jsonb NOT NULL,
 version bigint NOT NULL DEFAULT 1 CHECK(version>0),
 PRIMARY KEY(owner_id,id)
);
CREATE TABLE routine_applications (
 owner_id uuid NOT NULL,
 template_id text NOT NULL,
 local_date date NOT NULL,
 plan_id uuid NOT NULL,
 template_version bigint NOT NULL,
 created_at timestamptz NOT NULL,
 PRIMARY KEY(owner_id,template_id,local_date),
 FOREIGN KEY(owner_id,template_id) REFERENCES routine_templates(owner_id,id),
 FOREIGN KEY(owner_id,plan_id) REFERENCES day_plans(owner_id,id)
);
-- +goose Down
DROP TABLE routine_applications;
DROP TABLE routine_templates;
