alter table traffic_usages
add column if not exists billed_bytes bigint not null default 0;
