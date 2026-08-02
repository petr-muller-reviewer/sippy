package util

import (
	"fmt"
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/stretchr/testify/require"

	apitype "github.com/openshift/sippy/pkg/apis/api"
	v1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
)

func CreateProwJob(t *testing.T, dbc *db.DB, name, release string, variants []string) models.ProwJob {
	t.Helper()
	job := models.ProwJob{
		Name:     name,
		Release:  release,
		Variants: pq.StringArray(variants),
	}
	require.NoError(t, dbc.DB.Create(&job).Error, "creating ProwJob %q", name)
	return job
}

type ProwJobOption func(*models.ProwJob)

func WithKind(kind models.ProwKind) ProwJobOption {
	return func(j *models.ProwJob) {
		j.Kind = kind
	}
}

func CreateProwJobWithOptions(t *testing.T, dbc *db.DB, name, release string, variants []string, opts ...ProwJobOption) models.ProwJob {
	t.Helper()
	job := models.ProwJob{
		Name:     name,
		Release:  release,
		Variants: pq.StringArray(variants),
	}
	for _, opt := range opts {
		opt(&job)
	}
	require.NoError(t, dbc.DB.Create(&job).Error, "creating ProwJob %q", name)
	return job
}

type ProwJobRunOption func(*models.ProwJobRun)

func WithURL(url string) ProwJobRunOption {
	return func(r *models.ProwJobRun) { r.URL = url }
}

func CreateProwJobRun(t *testing.T, dbc *db.DB, prowJobID uint, release string, timestamp time.Time, succeeded bool, overallResult v1.JobOverallResult, opts ...ProwJobRunOption) models.ProwJobRun {
	t.Helper()
	run := models.ProwJobRun{
		ProwJobID:      prowJobID,
		ProwJobRelease: release,
		Timestamp:      timestamp,
		Succeeded:      succeeded,
		Failed:         !succeeded,
		OverallResult:  overallResult,
	}
	for _, opt := range opts {
		opt(&run)
	}
	require.NoError(t, dbc.DB.Create(&run).Error, "creating ProwJobRun")
	return run
}

func CreateTest(t *testing.T, dbc *db.DB, name string) models.Test {
	t.Helper()
	test := models.Test{Name: name}
	require.NoError(t, dbc.DB.Create(&test).Error, "creating Test %q", name)
	return test
}

func CreateSuite(t *testing.T, dbc *db.DB, name string) models.Suite {
	t.Helper()
	suite := models.Suite{Name: name}
	require.NoError(t, dbc.DB.Create(&suite).Error, "creating Suite %q", name)
	return suite
}

func CreateProwJobRunTest(t *testing.T, dbc *db.DB, prowJobRunID, prowJobID, testID uint, release string, timestamp time.Time, status int) models.ProwJobRunTest {
	t.Helper()
	pjrt := models.ProwJobRunTest{
		ProwJobRunID:        prowJobRunID,
		ProwJobID:           prowJobID,
		TestID:              testID,
		ProwJobRunRelease:   release,
		ProwJobRunTimestamp: timestamp,
		Status:              status,
	}
	require.NoError(t, dbc.DB.Create(&pjrt).Error, "creating ProwJobRunTest")
	return pjrt
}

func CreateBug(t *testing.T, dbc *db.DB, key, status, summary string, lastChangeTime time.Time, jobs []models.ProwJob) models.Bug {
	t.Helper()
	bug := models.Bug{
		Key:            key,
		Status:         status,
		Summary:        summary,
		LastChangeTime: lastChangeTime,
		Jobs:           jobs,
	}
	require.NoError(t, dbc.DB.Create(&bug).Error, "creating Bug %q", key)
	return bug
}

// ReleaseTagOption customizes a ReleaseTag before creation.
type ReleaseTagOption func(*models.ReleaseTag)

func WithPhase(phase string) ReleaseTagOption {
	return func(rt *models.ReleaseTag) { rt.Phase = phase }
}

func WithPreviousOSVersion(version string) ReleaseTagOption {
	return func(rt *models.ReleaseTag) { rt.PreviousOSVersion = version }
}

func WithCurrentOSVersion(version string) ReleaseTagOption {
	return func(rt *models.ReleaseTag) { rt.CurrentOSVersion = version }
}

func WithPreviousReleaseTag(prev string) ReleaseTagOption {
	return func(rt *models.ReleaseTag) { rt.PreviousReleaseTag = prev }
}

func WithForced(forced bool) ReleaseTagOption {
	return func(rt *models.ReleaseTag) { rt.Forced = forced }
}

func CreateReleaseTag(t *testing.T, dbc *db.DB, releaseTag, release, stream, arch string, releaseTime time.Time, opts ...ReleaseTagOption) models.ReleaseTag {
	t.Helper()
	rt := models.ReleaseTag{
		ReleaseTag:   releaseTag,
		Release:      release,
		Stream:       stream,
		Architecture: arch,
		Phase:        apitype.PayloadAccepted,
		ReleaseTime:  releaseTime,
	}
	for _, opt := range opts {
		opt(&rt)
	}
	require.NoError(t, dbc.DB.Create(&rt).Error, "creating ReleaseTag %q", releaseTag)
	return rt
}

type ReleasePullRequestOption func(*models.ReleasePullRequest)

func WithPullRequestID(id string) ReleasePullRequestOption {
	return func(pr *models.ReleasePullRequest) { pr.PullRequestID = id }
}

func WithBugURL(url string) ReleasePullRequestOption {
	return func(pr *models.ReleasePullRequest) { pr.BugURL = url }
}

func CreateReleasePullRequest(t *testing.T, dbc *db.DB, url, name, description string, opts ...ReleasePullRequestOption) models.ReleasePullRequest {
	t.Helper()
	pr := models.ReleasePullRequest{
		URL:         url,
		Name:        name,
		Description: description,
	}
	for _, opt := range opts {
		opt(&pr)
	}
	require.NoError(t, dbc.DB.Create(&pr).Error, "creating ReleasePullRequest %q", url)
	return pr
}

func CreateReleaseJobRun(t *testing.T, dbc *db.DB, releaseTagID, prowJobRunID uint, jobName, kind, state, url string) models.ReleaseJobRun {
	t.Helper()
	rjr := models.ReleaseJobRun{
		ReleaseTagID: fmt.Sprintf("%d", releaseTagID),
		Name:         prowJobRunID,
		JobName:      jobName,
		Kind:         kind,
		State:        state,
		URL:          url,
	}
	require.NoError(t, dbc.DB.Create(&rjr).Error, "creating ReleaseJobRun for tag %d", releaseTagID)
	return rjr
}

func LinkReleaseTagPullRequests(t *testing.T, dbc *db.DB, tag *models.ReleaseTag, prs ...models.ReleasePullRequest) {
	t.Helper()
	prPtrs := make([]*models.ReleasePullRequest, len(prs))
	for i := range prs {
		prPtrs[i] = &prs[i]
	}
	require.NoError(t, dbc.DB.Model(tag).Association("PullRequests").Append(prPtrs), "linking PRs to ReleaseTag %q", tag.ReleaseTag)
}
