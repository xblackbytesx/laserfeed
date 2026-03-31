-- Migrate 'extract' image_mode values: content extraction now always happens
-- automatically, so 'extract' is equivalent to 'none' (no extra final fallback).
UPDATE feeds SET image_mode = 'none' WHERE image_mode = 'extract';
UPDATE global_settings SET value = 'none' WHERE key = 'image_mode' AND value = 'extract';

-- Recreate the enum without the 'extract' value.
CREATE TYPE image_mode_new AS ENUM ('none', 'placeholder', 'random');

-- Drop the column default first (it references the old enum literal 'extract').
ALTER TABLE feeds ALTER COLUMN image_mode DROP DEFAULT;

ALTER TABLE feeds
    ALTER COLUMN image_mode TYPE image_mode_new
    USING image_mode::text::image_mode_new;

-- Restore the default using the new enum type.
ALTER TABLE feeds ALTER COLUMN image_mode SET DEFAULT 'none';

DROP TYPE image_mode;
ALTER TYPE image_mode_new RENAME TO image_mode;
