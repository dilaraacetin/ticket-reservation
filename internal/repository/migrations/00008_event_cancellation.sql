-- +goose Up

-- Nullable rather than a status column with a default: an event is either on
-- sale or it carries the moment it stopped being, and "when" is the part people
-- ask about afterwards.
alter table events
    add column cancelled_at timestamptz;

-- +goose Down

alter table events drop column cancelled_at;
