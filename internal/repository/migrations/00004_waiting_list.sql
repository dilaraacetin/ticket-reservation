-- +goose Up

create table waiting_list (
    id        text        primary key,
    event_id  text        not null references events (id) on delete cascade,
    user_id   text        not null,
    joined_at timestamptz not null,

    -- One place per person. This is what makes joining twice harmless rather
    -- than a way to occupy the queue several times over.
    constraint waiting_list_one_place_per_user unique (event_id, user_id)
);

-- The queue is only ever read front first, so it is indexed in the order it is
-- read in.
create index waiting_list_queue_order
    on waiting_list (event_id, joined_at, id);

-- +goose Down

drop table waiting_list;
