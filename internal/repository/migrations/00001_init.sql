-- +goose Up

create table events (
    id        text        primary key,
    name      text        not null,
    venue     text        not null,
    starts_at timestamptz not null
);

create table seats (
    event_id        text        not null references events (id) on delete cascade,
    id              text        not null,
    row_label       text        not null,
    number          integer     not null,
    status          text        not null,

    hold_id         text,
    held_by         text,
    hold_created_at timestamptz,
    hold_expires_at timestamptz,

    reserved_by     text,

    version         bigint      not null default 1,

    primary key (event_id, id),

    constraint seats_status_valid
        check (status in ('available', 'held', 'reserved')),


    constraint seats_hold_columns_consistent
        check (
            (status = 'held'
                and hold_id is not null
                and held_by is not null
                and hold_created_at is not null
                and hold_expires_at is not null)
            or
            (status <> 'held'
                and hold_id is null
                and held_by is null
                and hold_created_at is null
                and hold_expires_at is null)
        ),

    constraint seats_reserved_by_consistent
        check ((status = 'reserved') = (reserved_by is not null))
);

create unique index seats_hold_id_unique
    on seats (hold_id)
    where hold_id is not null;

create index seats_expired_holds
    on seats (hold_expires_at)
    where status = 'held';

-- +goose Down

drop table seats;
drop table events;
