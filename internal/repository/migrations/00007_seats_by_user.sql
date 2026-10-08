-- +goose Up

-- Until now seats were only ever read by event, so nothing could answer "what
-- does this person have". Two partial indexes rather than one over both
-- columns: a seat is held by somebody or reserved by somebody, never both, and
-- a partial index skips every row where the column is null — which is most of
-- them.
create index seats_held_by on seats (held_by) where held_by is not null;

create index seats_reserved_by on seats (reserved_by) where reserved_by is not null;

-- +goose Down

drop index seats_reserved_by;
drop index seats_held_by;
