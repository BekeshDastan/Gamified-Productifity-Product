-- +goose Up
CREATE TABLE score_versions (version text PRIMARY KEY,definition jsonb NOT NULL);
INSERT INTO score_versions VALUES('score-v1','{"daily":{"T":0.6,"H":0.4},"discipline":{"T":0.5,"H":0.35,"R":0.15},"lookback_days":14,"minimum_closed_days":3,"rounding":"half_up","rest":"excluded","empty":"null"}');
CREATE TRIGGER score_versions_immutable BEFORE UPDATE OR DELETE ON score_versions FOR EACH ROW EXECUTE FUNCTION reject_economy_rewrite();
-- +goose Down
DROP TABLE score_versions;
