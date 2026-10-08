-- +goose Up

-- Defaulted to 'other' rather than the empty string, because unlike the columns
-- beside it this one has a meaningful value for "nobody said": events stored
-- before the catalogue had categories are uncategorised, not blank.
alter table events
    add column category text not null default 'other';

alter table events
    add constraint events_category_valid
        check (category in
            ('concert', 'theatre', 'comedy', 'festival', 'sport', 'family', 'other'));

-- The two filters are used together, so they are indexed together.
create index events_category_city_starts_at_idx
    on events (category, city, starts_at);

-- +goose Down

drop index events_category_city_starts_at_idx;

alter table events drop constraint events_category_valid;

alter table events drop column category;
