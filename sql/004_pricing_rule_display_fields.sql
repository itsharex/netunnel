alter table if exists pricing_rules
    add column if not exists display_name varchar(128),
    add column if not exists description text;

update pricing_rules
set display_name = case name
    when 'default-traffic' then '按量流量包'
    when 'monthly-10g' then '包月 10G'
    when 'monthly-20g' then '包月 20G'
    when 'monthly-unlimited' then '包月不限量'
    when 'yearly-10g' then '包年 10G'
    when 'yearly-20g' then '包年 20G'
    when 'yearly-unlimited' then '包年不限量'
    else display_name
end
where display_name is null;

update pricing_rules
set description = case name
    when 'default-traffic' then '适合低频使用场景，按实际流量结算，长期有效。'
    when 'monthly-10g' then '适合轻量业务，每月含 10G 流量，超出后按量计费。'
    when 'monthly-20g' then '适合稳定业务，每月含 20G 流量，超出后按量计费。'
    when 'monthly-unlimited' then '适合持续在线业务，包月有效期内不限流量。'
    when 'yearly-10g' then '适合长期轻量业务，按年订阅，每月重置 10G 流量。'
    when 'yearly-20g' then '适合长期稳定业务，按年订阅，每月重置 20G 流量。'
    when 'yearly-unlimited' then '适合核心生产业务，按年订阅，全程不限流量。'
    else description
end
where description is null;
