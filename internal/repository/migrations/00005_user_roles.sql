-- +goose Up

-- Defaulted rather than nullable: every existing row becomes a customer, and a
-- row with no role at all is never something a check has to handle.
alter table users
    add column role text not null default 'customer';

-- The same closed set the domain parses. Without it a typo in a manual update
-- becomes a role that reads as if it grants something.
alter table users
    add constraint users_role_valid check (role in ('customer', 'admin'));

-- +goose Down

alter table users drop constraint users_role_valid;
alter table users drop column role;
