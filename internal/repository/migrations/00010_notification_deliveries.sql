-- +goose Up

-- An outbox. The notice itself is written once and is then simply true; sending
-- it reaches somebody else's server and may have to be tried again tomorrow, so
-- the two are kept apart.
create table notification_deliveries (
    id              text        primary key,
    notification_id text        not null references notifications (id) on delete cascade,
    user_id         text        not null,
    channel         text        not null,

    attempts        integer     not null default 0,
    last_error      text        not null default '',

    created_at      timestamptz not null,

    -- When this may next be tried. The worker reads by this column, and the
    -- wait doubles after each failure.
    due_at          timestamptz not null,

    sent_at         timestamptz,
    gave_up_at      timestamptz,

    constraint notification_deliveries_channel_valid check (channel in ('email', 'push')),

    -- One delivery per notice per channel. Without it a retried enqueue would
    -- send the same thing twice.
    constraint notification_deliveries_once unique (notification_id, channel)
);

-- The worker's only query: what is due and not finished with. Partial, because
-- everything already sent or given up on is dead weight in this index.
create index notification_deliveries_due
    on notification_deliveries (due_at)
    where sent_at is null and gave_up_at is null;

-- +goose Down

drop table notification_deliveries;
