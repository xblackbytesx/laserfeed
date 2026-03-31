-- Add 'builtin' image mode: uses one of LaserFeed's built-in placeholder SVGs.
CREATE TYPE image_mode_new AS ENUM ('none', 'placeholder', 'random', 'builtin');

ALTER TABLE feeds ALTER COLUMN image_mode DROP DEFAULT;

ALTER TABLE feeds
    ALTER COLUMN image_mode TYPE image_mode_new
    USING image_mode::text::image_mode_new;

ALTER TABLE feeds ALTER COLUMN image_mode SET DEFAULT 'none';

DROP TYPE image_mode;
ALTER TYPE image_mode_new RENAME TO image_mode;

-- Add the builtin_placeholder setting (stores the chosen SVG filename).
INSERT INTO global_settings (key, value, updated_at)
VALUES ('builtin_placeholder', 'laserfeed-placeholder.svg', NOW())
ON CONFLICT (key) DO NOTHING;
