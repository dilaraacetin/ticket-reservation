-- +goose Up

-- The descriptive half of an event. All four default to the empty string rather
-- than null: every one of them is read straight into a page, and a column that
-- can be either empty or absent makes every reader handle two kinds of nothing.
alter table events
    add column city        text not null default '',
    add column image_url   text not null default '',
    add column description text not null default '',
    add column rules       text not null default '';

-- The catalogue is browsed by city, and a filtered list is the first page most
-- visitors see.
create index events_city_starts_at_idx on events (city, starts_at);

-- +goose Down

drop index events_city_starts_at_idx;

alter table events
    drop column city,
    drop column image_url,
    drop column description,
    drop column rules;
