-- +goose Up

-- Only the exceptions are stored. A token is trusted because of its signature,
-- so the table holds what signing out refuses rather than what is valid, and it
-- stays small because a record is dropped once its token would have expired.
create table revoked_tokens (
    id         text        primary key,
    expires_at timestamptz not null
);

-- The sweeper deletes by expiry, and this is the only query that does not go by
-- primary key.
create index revoked_tokens_expiry on revoked_tokens (expires_at);

-- +goose Down

drop table revoked_tokens;
