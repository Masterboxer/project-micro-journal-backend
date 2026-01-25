Apply all pending migrations
`make migrate-up`

Roll back last migration
`make migrate-down`

Check current version
`make migrate-version`

Create a new migration
`make migrate-create name=add_journal_entries`

Create DB Backup
`docker exec -t backend-micro_journal_db-1 pg_dump -U postgres journal > backup.sql`

Run CRON Job Service
`docker compose run --rm daily-reminder-worker`
