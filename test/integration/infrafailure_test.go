package integration

import (
	"testing"
	"time"

	"cloud.google.com/go/civil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openshift/sippy/pkg/dataloader/prowloader/pgwriter"
	"github.com/openshift/sippy/pkg/db/infrafailure"
	"github.com/openshift/sippy/pkg/db/models"
	intutil "github.com/openshift/sippy/test/integration/util"
)

// TestRecordInfraFailureSubtractsFromSummaries writes two runs of the same test
// on the same day, marks one as an infrastructure failure, and verifies its
// contribution is removed from both the daily totals and the cumulative
// summaries while the other run remains counted.
func TestRecordInfraFailureSubtractsFromSummaries(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	jobID := seedProwJob(t, dbc, "periodic-e2e-aws", "4.18")
	today := testDate
	ts := time.Date(today.Year, today.Month, today.Day, 10, 0, 0, 0, time.UTC)

	const infraRunID = 40001
	const keepRunID = 40002

	writeBatch(t, dbc, testDate, []pgwriter.JobRunResult{
		{
			Run: pgwriter.RunRow{ID: infraRunID, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: ts},
			Tests: []pgwriter.TestRow{
				{ProwJobRunID: infraRunID, ProwJobID: jobID, ProwJobRunTimestamp: ts, ProwJobRunRelease: "4.18", TestName: "infra-sub-test", SuiteName: "junit_e2e", Status: statusSuccess, Duration: 1.0},
			},
		},
		{
			Run: pgwriter.RunRow{ID: keepRunID, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: ts},
			Tests: []pgwriter.TestRow{
				{ProwJobRunID: keepRunID, ProwJobID: jobID, ProwJobRunTimestamp: ts, ProwJobRunRelease: "4.18", TestName: "infra-sub-test", SuiteName: "junit_e2e", Status: statusFailure, Duration: 2.0},
			},
		},
	})

	var test models.Test
	require.NoError(t, dbc.DB.Where("name = ?", "infra-sub-test").First(&test).Error)

	// Precondition: both runs counted (1 success + 1 failure = 2 runs).
	var dt models.TestDailyTotal
	require.NoError(t, dbc.DB.Where("test_id = ? AND prow_job_id = ? AND release = ? AND date = ?", test.ID, jobID, "4.18", today).First(&dt).Error)
	require.Equal(t, int32(1), dt.Successes)
	require.Equal(t, int32(1), dt.Failures)
	require.Equal(t, int32(2), dt.Runs)

	// Record the infra failure for the first run. RecordInfraFailure opens its
	// own transaction on the supplied connection.
	require.NoError(t, infrafailure.RecordInfraFailure(dbc.DB, infraRunID))

	// The InfraFailure label is applied to the infra run (exercising the
	// NULL-safe gate, since the run had no labels).
	var infraRun models.ProwJobRun
	require.NoError(t, dbc.DB.First(&infraRun, infraRunID).Error)
	assert.Contains(t, []string(infraRun.Labels), infrafailure.LabelInfraFailure)

	// Daily totals now reflect only the retained run's failure.
	require.NoError(t, dbc.DB.Where("test_id = ? AND prow_job_id = ? AND release = ? AND date = ?", test.ID, jobID, "4.18", today).First(&dt).Error)
	assert.Equal(t, int32(0), dt.Successes, "infra run's success should be subtracted")
	assert.Equal(t, int32(1), dt.Failures, "retained run's failure should remain")
	assert.Equal(t, int32(1), dt.Runs)

	// Cumulative summaries cascade the subtraction from the affected date onward
	// (today and the carried-forward tomorrow row).
	tomorrow := today.AddDays(1)
	for _, d := range []civil.Date{today, tomorrow} {
		var cs models.TestCumulativeSummary
		require.NoError(t, dbc.DB.Where("test_id = ? AND prow_job_id = ? AND release = ? AND date = ?", test.ID, jobID, "4.18", d).First(&cs).Error, "date %s", d)
		assert.Equal(t, int64(0), cs.PrefixSumSuccesses, "date %s", d)
		assert.Equal(t, int64(1), cs.PrefixSumFailures, "date %s", d)
		assert.Equal(t, int64(1), cs.PrefixSumRuns, "date %s", d)
	}
}

// TestRecordInfraFailureIsIdempotent verifies that repeated calls for the same
// run subtract exactly once and never drive the totals negative or duplicate
// the label.
func TestRecordInfraFailureIsIdempotent(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	jobID := seedProwJob(t, dbc, "periodic-e2e-aws", "4.18")
	today := testDate
	ts := time.Date(today.Year, today.Month, today.Day, 10, 0, 0, 0, time.UTC)

	const runID = 41001

	writeBatch(t, dbc, testDate, []pgwriter.JobRunResult{{
		Run: pgwriter.RunRow{ID: runID, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: ts},
		Tests: []pgwriter.TestRow{
			{ProwJobRunID: runID, ProwJobID: jobID, ProwJobRunTimestamp: ts, ProwJobRunRelease: "4.18", TestName: "infra-idem-test", SuiteName: "junit_e2e", Status: statusSuccess, Duration: 1.0},
		},
	}})

	var test models.Test
	require.NoError(t, dbc.DB.Where("name = ?", "infra-idem-test").First(&test).Error)

	call := func() {
		require.NoError(t, infrafailure.RecordInfraFailure(dbc.DB, runID))
	}

	// First call subtracts the single run down to zero.
	call()
	var dt models.TestDailyTotal
	require.NoError(t, dbc.DB.Where("test_id = ? AND prow_job_id = ? AND release = ? AND date = ?", test.ID, jobID, "4.18", today).First(&dt).Error)
	assert.Equal(t, int32(0), dt.Successes)
	assert.Equal(t, int32(0), dt.Runs)

	// Subsequent calls must be no-ops.
	call()
	call()
	require.NoError(t, dbc.DB.Where("test_id = ? AND prow_job_id = ? AND release = ? AND date = ?", test.ID, jobID, "4.18", today).First(&dt).Error)
	assert.Equal(t, int32(0), dt.Successes, "idempotent: no double subtraction")
	assert.Equal(t, int32(0), dt.Runs, "idempotent: totals not driven negative")

	// Label present exactly once.
	var run models.ProwJobRun
	require.NoError(t, dbc.DB.First(&run, runID).Error)
	labelCount := 0
	for _, l := range run.Labels {
		if l == infrafailure.LabelInfraFailure {
			labelCount++
		}
	}
	assert.Equal(t, 1, labelCount, "InfraFailure label should be applied exactly once")
}

// TestCreateBatchDeltasExcludesInfraFailureRuns verifies the write-time
// exclusion: a run that already carries the InfraFailure label when the batch
// is written never contributes to the summary tables.
func TestCreateBatchDeltasExcludesInfraFailureRuns(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	jobID := seedProwJob(t, dbc, "periodic-e2e-aws", "4.18")
	today := testDate
	ts := time.Date(today.Year, today.Month, today.Day, 10, 0, 0, 0, time.UTC)

	writeBatch(t, dbc, testDate, []pgwriter.JobRunResult{
		{
			Run: pgwriter.RunRow{ID: 42001, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: ts, Labels: []string{infrafailure.LabelInfraFailure}},
			Tests: []pgwriter.TestRow{
				{ProwJobRunID: 42001, ProwJobID: jobID, ProwJobRunTimestamp: ts, ProwJobRunRelease: "4.18", TestName: "write-exclude-test", SuiteName: "junit_e2e", Status: statusSuccess, Duration: 1.0},
			},
		},
		{
			Run: pgwriter.RunRow{ID: 42002, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: ts},
			Tests: []pgwriter.TestRow{
				{ProwJobRunID: 42002, ProwJobID: jobID, ProwJobRunTimestamp: ts, ProwJobRunRelease: "4.18", TestName: "write-exclude-test", SuiteName: "junit_e2e", Status: statusFailure, Duration: 2.0},
			},
		},
	})

	var test models.Test
	require.NoError(t, dbc.DB.Where("name = ?", "write-exclude-test").First(&test).Error)

	var dt models.TestDailyTotal
	require.NoError(t, dbc.DB.Where("test_id = ? AND prow_job_id = ? AND release = ? AND date = ?", test.ID, jobID, "4.18", today).First(&dt).Error)
	// Only the non-infra run (a failure) is counted; the infra-labeled run's
	// success is excluded.
	assert.Equal(t, int32(0), dt.Successes, "infra-labeled run's success should be excluded")
	assert.Equal(t, int32(1), dt.Failures)
	assert.Equal(t, int32(1), dt.Runs)
}
