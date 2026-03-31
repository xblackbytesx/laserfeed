-- Restore the 'extract' value to the image_mode enum.
CREATE TYPE image_mode_old AS ENUM ('none', 'extract', 'placeholder', 'random');

ALTER TABLE feeds ALTER COLUMN image_mode DROP DEFAULT;

ALTER TABLE feeds
    ALTER COLUMN image_mode TYPE image_mode_old
    USING image_mode::text::image_mode_old;

ALTER TABLE feeds ALTER COLUMN image_mode SET DEFAULT 'extract';

DROP TYPE image_mode;
ALTER TYPE image_mode_old RENAME TO image_mode;

-- Restore global_settings default (was 'extract' before migration).
UPDATE global_settings SET value = 'extract' WHERE key = 'image_mode' AND value = 'none';
