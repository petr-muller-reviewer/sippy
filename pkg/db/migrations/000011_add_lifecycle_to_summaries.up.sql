-- TRT-2752: Add lifecycle as a key column in summary tables.
-- Separates blocking/informing test results in daily and cumulative
-- aggregations so downstream queries (CR, test reports) can filter by
-- lifecycle. Existing rows default to 'blocking'.

ALTER TABLE test_daily_totals
    ADD COLUMN IF NOT EXISTS lifecycle TEXT NOT NULL DEFAULT 'blocking';

ALTER TABLE test_cumulative_summaries
    ADD COLUMN IF NOT EXISTS lifecycle TEXT NOT NULL DEFAULT 'blocking';

DROP INDEX IF EXISTS idx_test_daily_totals_unique;
ALTER TABLE test_daily_totals
    ADD PRIMARY KEY (release, date, test_id, prow_job_id, suite_id, lifecycle);

ALTER TABLE test_cumulative_summaries
    DROP CONSTRAINT IF EXISTS test_cumulative_summaries_pkey;
ALTER TABLE test_cumulative_summaries
    ADD PRIMARY KEY (release, date, test_id, prow_job_id, suite_id, lifecycle);
