-- articles テーブルへの INSERT 時に NOTIFY を発行するトリガー
CREATE OR REPLACE FUNCTION notify_new_article() RETURNS trigger AS $$
BEGIN
    PERFORM pg_notify('new_article', json_build_object(
        'id', NEW.id,
        'source_type', NEW.source_type,
        'title', NEW.title
    )::text);
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_new_article ON articles;
CREATE TRIGGER trg_new_article
    AFTER INSERT ON articles
    FOR EACH ROW
    EXECUTE FUNCTION notify_new_article();
