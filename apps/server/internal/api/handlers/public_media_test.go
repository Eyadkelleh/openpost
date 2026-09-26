package handlers

import (
	"context"
	"testing"
	"time"

	"github.com/openpost/backend/internal/models"
	"github.com/openpost/backend/internal/services/publicurl"
	"github.com/stretchr/testify/require"
)

type fixedPublicMediaVerifier struct {
	result publicurl.Result
	url    string
}

func (v *fixedPublicMediaVerifier) Verify(_ context.Context, rawURL string) publicurl.Result {
	v.url = rawURL
	return v.result
}

func TestRefreshPublicMediaStateRepairsLegacyRelativeURLFailure(t *testing.T) {
	db := createHandlerTestDB(t, (*models.MediaAttachment)(nil))
	checkedAt := time.Now().UTC().Add(-time.Minute)
	media := models.MediaAttachment{
		ID:                 "media-1",
		WorkspaceID:        "workspace-1",
		FilePath:           "media-1.jpg",
		StorageType:        "local",
		MimeType:           "image/jpeg",
		Size:               1024,
		OriginalFilename:   "image.jpg",
		PublicURLCheckedAt: checkedAt,
		PublicURLError:     "public media URL must use HTTPS",
	}
	_, err := db.NewInsert().Model(&media).Exec(t.Context())
	require.NoError(t, err)

	resultCheckedAt := time.Now().UTC()
	fixed := &fixedPublicMediaVerifier{result: publicurl.Result{
		Ready:      true,
		StatusCode: 200,
		CheckedAt:  resultCheckedAt,
	}}
	verifier := publicurl.NewMediaVerifier("https://app.openpost.test/media", nil, nil)
	verifier.SetVerifier(fixed)

	err = refreshPublicMediaState(t.Context(), db, verifier, &media)

	require.NoError(t, err)
	require.True(t, media.PublicURLReady)
	require.Equal(t, "https://app.openpost.test/media/media-1.jpg", fixed.url)
	var persisted models.MediaAttachment
	require.NoError(t, db.NewSelect().Model(&persisted).Where("id = ?", media.ID).Scan(t.Context()))
	require.True(t, persisted.PublicURLReady)
	require.Equal(t, 200, persisted.PublicURLStatus)
	require.Empty(t, persisted.PublicURLError)
}

func TestRefreshPublicMediaStateAfterUploadRecordsPendingOnTransient404(t *testing.T) {
	db := createHandlerTestDB(t, (*models.MediaAttachment)(nil))
	media := models.MediaAttachment{
		ID:               "media-pending",
		WorkspaceID:      "workspace-1",
		FilePath:         "media-pending.jpg",
		StorageType:      "local",
		MimeType:         "image/jpeg",
		Size:             1024,
		OriginalFilename: "image.jpg",
	}
	_, err := db.NewInsert().Model(&media).Exec(t.Context())
	require.NoError(t, err)

	fixed := &fixedPublicMediaVerifier{result: publicurl.Result{
		StatusCode: 404,
		Error:      "public media URL returned 404",
		CheckedAt:  time.Now().UTC(),
	}}
	verifier := publicurl.NewMediaVerifier("https://app.openpost.test/media", nil, nil)
	verifier.SetVerifier(fixed)

	require.NoError(t, refreshPublicMediaStateAfterUpload(t.Context(), db, verifier, &media))
	require.False(t, media.PublicURLReady)
	require.Equal(t, 0, media.PublicURLStatus)
	require.Empty(t, media.PublicURLError)
	require.False(t, media.PublicURLCheckedAt.IsZero())

	var persisted models.MediaAttachment
	require.NoError(t, db.NewSelect().Model(&persisted).Where("id = ?", media.ID).Scan(t.Context()))
	require.False(t, persisted.PublicURLReady)
	require.Equal(t, 0, persisted.PublicURLStatus)
	require.Empty(t, persisted.PublicURLError)
}

func TestRefreshPublicMediaStateRecoversStored404WithoutManualRetry(t *testing.T) {
	db := createHandlerTestDB(t, (*models.MediaAttachment)(nil))
	media := models.MediaAttachment{
		ID:                 "media-stale-404",
		WorkspaceID:        "workspace-1",
		FilePath:           "media-stale-404.jpg",
		StorageType:        "local",
		MimeType:           "image/jpeg",
		Size:               1024,
		OriginalFilename:   "image.jpg",
		PublicURLReady:     false,
		PublicURLStatus:    404,
		PublicURLError:     "public media URL returned 404",
		PublicURLCheckedAt: time.Now().UTC().Add(-2 * time.Minute),
	}
	_, err := db.NewInsert().Model(&media).Exec(t.Context())
	require.NoError(t, err)

	fixed := &fixedPublicMediaVerifier{result: publicurl.Result{
		Ready:      true,
		StatusCode: 200,
		CheckedAt:  time.Now().UTC(),
	}}
	verifier := publicurl.NewMediaVerifier("https://app.openpost.test/media", nil, nil)
	verifier.SetVerifier(fixed)

	require.NoError(t, refreshPublicMediaState(t.Context(), db, verifier, &media))
	require.True(t, media.PublicURLReady)
	require.Equal(t, 200, media.PublicURLStatus)

	var persisted models.MediaAttachment
	require.NoError(t, db.NewSelect().Model(&persisted).Where("id = ?", media.ID).Scan(t.Context()))
	require.True(t, persisted.PublicURLReady)
	require.Equal(t, 200, persisted.PublicURLStatus)
	require.Empty(t, persisted.PublicURLError)
}
