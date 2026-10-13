-- +goose Up
CREATE TABLE cosmetic_items (id text PRIMARY KEY, slot text NOT NULL CHECK(slot IN('base','clothes','head','accessory','background','frame')),catalog_version text NOT NULL,snapshot jsonb NOT NULL);

CREATE TABLE character_profiles(owner_id uuid PRIMARY KEY REFERENCES users(id),version bigint NOT NULL DEFAULT 1 CHECK(version>0),equipment jsonb NOT NULL,title_id text NOT NULL DEFAULT 'level:auto',updated_at timestamptz NOT NULL);
CREATE TABLE character_inventory(owner_id uuid NOT NULL REFERENCES users(id),item_id text NOT NULL REFERENCES cosmetic_items(id),active boolean NOT NULL,source text NOT NULL,granted_at timestamptz NOT NULL,revoked_at timestamptz,PRIMARY KEY(owner_id,item_id));
CREATE TABLE character_inventory_events(id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,owner_id uuid NOT NULL,item_id text NOT NULL,active boolean NOT NULL,reason text NOT NULL,created_at timestamptz NOT NULL,FOREIGN KEY(owner_id,item_id) REFERENCES character_inventory(owner_id,item_id));
-- +goose Down
DROP TABLE character_inventory_events;
DROP TABLE character_inventory;
DROP TABLE character_profiles;
DROP TABLE cosmetic_items;
