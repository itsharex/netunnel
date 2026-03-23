alter table if exists domain_routes
    drop constraint if exists domain_routes_cert_id_fkey;

alter table if exists domain_routes
    drop constraint if exists chk_domain_routes_cert_source;

alter table if exists domain_routes
    drop column if exists cert_source,
    drop column if exists cert_id;

drop index if exists idx_certificates_user_id;

drop table if exists certificates;
