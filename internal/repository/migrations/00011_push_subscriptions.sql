-- +goose Up

create table push_subscriptions (
    id         text        primary key,
    user_id    text        not null,

    -- The URL the browser's push service listens on, and what identifies the
    -- subscription: the same browser re-subscribing produces the same endpoint,
    -- so it is the key rather than the id.
    endpoint   text        not null,

    p256dh     text        not null,
    auth       text        not null,
    created_at timestamptz not null,

    constraint push_subscriptions_endpoint_unique unique (endpoint)
);

create index push_subscriptions_for_user on push_subscriptions (user_id);

-- +goose Down

drop table push_subscriptions;
