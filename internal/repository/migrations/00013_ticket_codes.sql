-- +goose Up

-- What is shown and scanned at the door. Null until the seat is confirmed, and
-- unique because two tickets that read the same are two tickets nobody can tell
-- apart at the gate.
alter table seats
    add column ticket_code text;

create unique index seats_ticket_code_unique
    on seats (ticket_code)
    where ticket_code is not null;

-- The gate reads by this and nothing else.
-- (the unique index above already serves that lookup)

-- +goose Down

drop index seats_ticket_code_unique;
alter table seats drop column ticket_code;
