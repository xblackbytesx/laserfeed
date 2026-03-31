ALTER TABLE feeds
  ADD COLUMN retention_max_items INT NOT NULL DEFAULT 0,
  ADD COLUMN retention_max_hours INT NOT NULL DEFAULT 0;
