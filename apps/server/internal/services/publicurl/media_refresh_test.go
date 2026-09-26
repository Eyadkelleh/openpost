package publicurl

import (
	"net/http"
	"testing"
	"time"

	"github.com/openpost/backend/internal/models"
	"github.com/stretchr/testify/require"
)

func TestIsTransientFailure(t *testing.T) {
	t.Parallel()

	require.False(t, IsTransientFailure(Result{Ready: true, StatusCode: http.StatusOK}))
	require.False(t, IsTransientFailure(Result{Error: MediaURLConfigurationError}))
	require.False(t, IsTransientFailure(Result{Error: legacyMediaURLError}))
	require.False(t, IsTransientFailure(Result{StatusCode: http.StatusForbidden, Error: "public media URL returned 403"}))
	require.True(t, IsTransientFailure(Result{StatusCode: http.StatusNotFound, Error: "public media URL returned 404"}))
	require.True(t, IsTransientFailure(Result{Error: "connection refused"}))
	require.True(t, IsTransientFailure(Result{StatusCode: http.StatusBadGateway, Error: "public media URL returned 502"}))
}

func TestNeedsRefreshRetriesFailedChecksAfterOneMinute(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	verifier := NewMediaVerifier("https://app.openpost.test/media", nil, nil)
	verifier.now = func() time.Time { return now }

	freshFailure := models.MediaAttachment{
		PublicURLReady:     false,
		PublicURLStatus:    http.StatusNotFound,
		PublicURLCheckedAt: now.Add(-30 * time.Second),
	}
	require.False(t, verifier.NeedsRefresh(freshFailure))

	staleFailure := freshFailure
	staleFailure.PublicURLCheckedAt = now.Add(-time.Minute)
	require.True(t, verifier.NeedsRefresh(staleFailure))

	ready := models.MediaAttachment{
		PublicURLReady:     true,
		PublicURLStatus:    http.StatusOK,
		PublicURLCheckedAt: now.Add(-2 * time.Hour),
	}
	require.False(t, verifier.NeedsRefresh(ready))
}
