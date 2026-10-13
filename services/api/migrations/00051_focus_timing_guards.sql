-- +goose Up
ALTER TABLE focus_sessions ADD COLUMN phase_remaining_seconds bigint CHECK(phase_remaining_seconds BETWEEN 0 AND 7200);
-- +goose StatementBegin
CREATE FUNCTION protect_focus_segment() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' OR OLD.ended_at IS NOT NULL OR
 ROW(OLD.id,OLD.owner_id,OLD.session_id,OLD.kind,OLD.started_at)
 IS DISTINCT FROM ROW(NEW.id,NEW.owner_id,NEW.session_id,NEW.kind,NEW.started_at)
 THEN RAISE EXCEPTION 'timing segments cannot be rewritten'; END IF;
 RETURN NEW;
END $$;
CREATE FUNCTION protect_focus_session() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' OR OLD.state IN('finished','cancelled') OR
 ROW(OLD.owner_id,OLD.id,OLD.request,OLD.mode,OLD.title,OLD.quest_id,OLD.goal_id,OLD.area_id,OLD.timezone,OLD.work_minutes,OLD.break_minutes,OLD.sound_enabled,OLD.started_at)
 IS DISTINCT FROM ROW(NEW.owner_id,NEW.id,NEW.request,NEW.mode,NEW.title,NEW.quest_id,NEW.goal_id,NEW.area_id,NEW.timezone,NEW.work_minutes,NEW.break_minutes,NEW.sound_enabled,NEW.started_at)
 THEN RAISE EXCEPTION 'focus identity and terminal result are permanent'; END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER focus_segment_immutable BEFORE UPDATE OR DELETE ON focus_segments FOR EACH ROW EXECUTE FUNCTION protect_focus_segment();
CREATE TRIGGER focus_session_immutable BEFORE UPDATE OR DELETE ON focus_sessions FOR EACH ROW EXECUTE FUNCTION protect_focus_session();
-- +goose Down
DROP TRIGGER focus_session_immutable ON focus_sessions;
DROP TRIGGER focus_segment_immutable ON focus_segments;
DROP FUNCTION protect_focus_session();
DROP FUNCTION protect_focus_segment();
ALTER TABLE focus_sessions DROP COLUMN phase_remaining_seconds;
