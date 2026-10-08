-- +goose Up

-- Null until somebody has followed a link sent to the address. Existing rows
-- stay null: they were never verified, and pretending otherwise would be the
-- migration quietly asserting something nobody checked.
alter table users
    add column email_verified_at timestamptz;

create table email_verifications (
    -- The token as it was emailed, not a hash of it.
    --
    -- Hashing would stop a read-only leak from yielding a working link, but it
    -- would also make the token unrecoverable at send time, which means sending
    -- inside the request that registered the account. What keeps the exposure
    -- small instead is that the row is deleted the moment the link is followed
    -- and swept once it expires: this table holds only live, unused tokens,
    -- each for at most a day.
    token      text        primary key,

    user_id    text        not null,

    -- The address the link was sent to, so that verifying an address the
    -- account has since changed away from does not verify the new one.
    email      text        not null,

    created_at timestamptz not null,
    expires_at timestamptz not null,

    -- Null until the message has gone. This is the sending queue.
    sent_at    timestamptz
);

-- The sender's only query: what still has to go out.
create index email_verifications_unsent
    on email_verifications (created_at)
    where sent_at is null;

create index email_verifications_for_user on email_verifications (user_id);

-- Swept by expiry.
create index email_verifications_expiry on email_verifications (expires_at);

-- +goose Down

drop table email_verifications;
alter table users drop column email_verified_at;
