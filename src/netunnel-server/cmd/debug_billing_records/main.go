package main

import (
	"database/sql"
	"fmt"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	userID := ""
	if len(os.Args) > 1 {
		userID = os.Args[1]
	}

	db, err := sql.Open("pgx", "postgresql://ai_ssh_user:ai_ssh_password@127.0.0.1:5432/netunnel")
	if err != nil {
		panic(err)
	}
	defer db.Close()

	if userID == "" {
		if err := db.QueryRow(`
select user_id
from user_subscriptions
where status = 'active'
  and cancelled_at is null
  and started_at <= now()
  and (expires_at is null or expires_at > now())
order by updated_at desc
limit 1`).Scan(&userID); err != nil {
			panic(err)
		}
	}

	fmt.Printf("USER %s\n", userID)

	fmt.Println("== ACTIVE SUBSCRIPTION ==")
	rows, err := db.Query(`
select
	us.id,
	us.status,
	us.started_at,
	us.current_period_start,
	us.current_period_end,
	us.current_period_used_bytes,
	us.expires_at,
	pr.name,
	pr.display_name,
	pr.billing_mode,
	pr.included_traffic_bytes,
	pr.is_unlimited
from user_subscriptions us
join pricing_rules pr on pr.id = us.pricing_rule_id
where us.user_id = $1
order by us.created_at desc
limit 5`, userID)
	if err != nil {
		panic(err)
	}
	for rows.Next() {
		var id, status, ruleName, displayName, billingMode string
		var startedAt, currentStart timeNull
		var currentEnd, expiresAt timeNull
		var usedBytes, includedBytes int64
		var isUnlimited bool
		if err := rows.Scan(&id, &status, &startedAt, &currentStart, &currentEnd, &usedBytes, &expiresAt, &ruleName, &displayName, &billingMode, &includedBytes, &isUnlimited); err != nil {
			panic(err)
		}
		fmt.Printf("subscription=%s status=%s started=%s current_start=%s current_end=%s used=%d expires=%s rule=%s(%s) mode=%s included=%d unlimited=%v\n",
			id, status, startedAt, currentStart, currentEnd, usedBytes, expiresAt, displayName, ruleName, billingMode, includedBytes, isUnlimited)
	}
	rows.Close()

	fmt.Println("== BUSINESS RECORDS ==")
	rows, err = db.Query(`
select
	id,
	record_type,
	amount::text,
	balance_before::text,
	balance_after::text,
	traffic_bytes,
	billable_bytes,
	package_expires_at,
	description,
	created_at
from user_business_records
where user_id = $1
order by created_at desc
limit 20`, userID)
	if err != nil {
		panic(err)
	}
	for rows.Next() {
		var id, recordType, changeAmount, before, after, description string
		var trafficBytes, billableBytes int64
		var packageExpiresAt, createdAt timeNull
		if err := rows.Scan(&id, &recordType, &changeAmount, &before, &after, &trafficBytes, &billableBytes, &packageExpiresAt, &description, &createdAt); err != nil {
			panic(err)
		}
		fmt.Printf("record=%s type=%s change=%s before=%s after=%s traffic_bytes=%d billable_bytes=%d package_expires=%s created=%s desc=%s\n",
			id, recordType, changeAmount, before, after, trafficBytes, billableBytes, packageExpiresAt, createdAt, description)
	}
	rows.Close()
}

type timeNull struct {
	sql.NullTime
}

func (t timeNull) String() string {
	if !t.Valid {
		return "-"
	}
	return t.Time.Format("2006-01-02 15:04:05")
}
