-- +goose Up

create table idempotency_keys (
    user_id     text        not null,
    key         text        not null,

    fingerprint text        not null,

    state       text        not null,
    status_code integer,
    response    bytea,

    created_at  timestamptz not null,
    expires_at  timestamptz not null,

    primary key (user_id, key),

    constraint idempotency_state_valid
        check (state in ('in_progress', 'completed')),

    constraint idempotency_response_consistent
        check (
            (state = 'completed'   and status_code is not null and response is not null)
            or
            (state = 'in_progress' and status_code is null     and response is null)
        )
);

create index idempotency_keys_expires_at on idempotency_keys (expires_at);

-- +goose Down

drop table idempotency_keys;
