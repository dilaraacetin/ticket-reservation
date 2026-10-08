-- +goose Up

create table notifications (
    id         text        primary key,
    user_id    text        not null,
    kind       text        not null,
    event_id   text        not null,

    -- Copied in rather than joined on. The event may be deleted later, and
    -- "your ticket to something" is not a message worth keeping.
    event_name text        not null,

    -- Empty when the notice is not about one seat, which is how somebody on the
    -- waiting list is told.
    seat_id    text        not null default '',

    created_at timestamptz not null,
    read_at    timestamptz,

    -- The same closed set the domain parses.
    constraint notifications_kind_valid check (kind in ('event_cancelled'))
);

-- Read newest first, per person. Nothing reads them any other way.
create index notifications_for_user on notifications (user_id, created_at desc);

-- No foreign key to events on purpose: a notice has to outlive the event it is
-- about, or withdrawing something would erase the telling of it.

-- +goose Down

drop table notifications;
