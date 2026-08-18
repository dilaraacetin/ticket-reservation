-- +goose Up

create table users (
    id            text        primary key,
    email         text        not null,
    password_hash text        not null,
    created_at    timestamptz not null,

    constraint users_email_unique unique (email)
);

-- +goose Down

drop table users;
