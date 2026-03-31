-- Migrate 'builtin' back to 'none' and remove the enum value.
UPDATE feeds SET image_mode = 'none' WHERE image_mode = 'builtin';

CREATE TYPE image_mode_old AS ENUM ('none', 'placeholder', 'random');

ALTER TABLE feeds ALTER COLUMN image_mode DROP DEFAULT;

ALTER TABLE feeds
    ALTER COLUMN image_mode TYPE image_mode_old
    USING image_mode::text::image_mode_old;

ALTER TABLE feeds ALTER COLUMN image_mode SET DEFAULT 'none';

DROP TYPE image_mode;
ALTER TYPE image_mode_old RENAME TO image_mode;

DELETE FROM global_settings WHERE key = 'builtin_placeholder';
