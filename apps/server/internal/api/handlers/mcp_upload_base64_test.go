package handlers

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"testing"
	"time"

	"github.com/openpost/backend/internal/models"
	"github.com/openpost/backend/internal/services/publicurl"
	"github.com/stretchr/testify/require"
)

func mcpTestPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 200, G: 40, B: 40, A: 255})
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}

func (s *mcpTestServer) executeOperation(t *testing.T, token, operation string, arguments map[string]any) map[string]any {
	t.Helper()
	resp := s.request(t, token, map[string]any{
		"jsonrpc": "2.0",
		"id":      "exec-" + operation,
		"method":  "tools/call",
		"params": map[string]any{
			"name": mcpToolExecute,
			"arguments": map[string]any{
				"operation": operation,
				"arguments": arguments,
			},
		},
	})
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	var out map[string]any
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &out))
	return out
}

func (s *mcpTestServer) queryOperation(t *testing.T, token, operation string, arguments map[string]any) map[string]any {
	t.Helper()
	resp := s.request(t, token, map[string]any{
		"jsonrpc": "2.0",
		"id":      "query-" + operation,
		"method":  "tools/call",
		"params": map[string]any{
			"name": mcpToolQuery,
			"arguments": map[string]any{
				"operation": operation,
				"arguments": arguments,
			},
		},
	})
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	var out map[string]any
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &out))
	return out
}

func (s *mcpTestServer) setPublicMediaVerifier(t *testing.T, verifier publicurl.Verifier) {
	t.Helper()
	media := publicurl.NewMediaVerifier("https://app.openpost.test/media", nil, nil)
	media.SetVerifier(verifier)
	s.handler.SetPublicMediaVerifier(media)
}

func TestMCPSearchOperationsListsUploadMediaBase64(t *testing.T) {
	t.Parallel()

	result, rpcErr := searchMCPOperations(map[string]any{"query": "upload media base64"})
	require.Nil(t, rpcErr)
	operations := result.(map[string]any)["structuredContent"].(map[string]any)["operations"].([]map[string]any)
	names := make([]string, 0, len(operations))
	for _, operation := range operations {
		names = append(names, operation["name"].(string))
	}
	require.Contains(t, names, mcpToolUploadBase64)

	srv := newMCPTestServer(t)
	resp := srv.request(t, "web-token", map[string]any{
		"jsonrpc": "2.0",
		"id":      "search-base64-upload",
		"method":  "tools/call",
		"params": map[string]any{
			"name":      mcpToolSearch,
			"arguments": map[string]any{"query": "upload a local file"},
		},
	})
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	var out map[string]any
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &out))
	require.NotContains(t, out, "error")
	listed := out["result"].(map[string]any)["structuredContent"].(map[string]any)["operations"].([]any)
	found := false
	for _, item := range listed {
		if item.(map[string]any)["name"] == mcpToolUploadBase64 {
			found = true
			break
		}
	}
	require.True(t, found, "search_operations should list upload_media_base64, got %#v", listed)
}

func TestMCPUploadMediaBase64CreatesWorkspaceMedia(t *testing.T) {
	t.Parallel()

	srv := newMCPTestServer(t)
	pngBytes := mcpTestPNG(t)
	out := srv.executeOperation(t, "web-token", mcpToolUploadBase64, map[string]any{
		"workspace_id":   "ws-1",
		"filename":       "flyer.png",
		"mime_type":      "image/png",
		"content_base64": base64.StdEncoding.EncodeToString(pngBytes),
		"alt_text":       "Launch flyer",
	})
	require.NotContains(t, out, "error", out)
	result := out["result"].(map[string]any)
	requireMCPStructuredJSONText(t, result, result["content"].([]any)[0].(map[string]any)["text"].(string))
	media := result["structuredContent"].(map[string]any)["media"].(map[string]any)
	require.NotEmpty(t, media["id"])
	require.Equal(t, "image/png", media["mime_type"])
	require.Equal(t, "flyer.png", media["filename"])
	require.Equal(t, "Launch flyer", media["alt_text"])
	require.Equal(t, "ready", media["processing_status"])
	require.Equal(t, "/media/"+media["id"].(string), media["url"])
	require.Equal(t, float64(len(pngBytes)), media["size"])

	var stored models.MediaAttachment
	require.NoError(t, srv.db.NewSelect().Model(&stored).Where("id = ?", media["id"]).Scan(t.Context()))
	require.Equal(t, "ws-1", stored.WorkspaceID)
	require.Equal(t, "image/png", stored.MimeType)
	require.Equal(t, "ready", stored.ProcessingStatus)
	require.Equal(t, "flyer.png", stored.OriginalFilename)
	require.Equal(t, int64(len(pngBytes)), stored.Size)
}

func TestMCPUploadMediaBase64AcceptsDataURL(t *testing.T) {
	t.Parallel()

	srv := newMCPTestServer(t)
	pngBytes := mcpTestPNG(t)
	out := srv.executeOperation(t, "web-token", mcpToolUploadBase64, map[string]any{
		"workspace_id":   "ws-1",
		"filename":       "card.png",
		"content_base64": "data:image/png;base64," + base64.StdEncoding.EncodeToString(pngBytes),
	})
	require.NotContains(t, out, "error", out)
	media := out["result"].(map[string]any)["structuredContent"].(map[string]any)["media"].(map[string]any)
	require.Equal(t, "image/png", media["mime_type"])
	require.Equal(t, float64(len(pngBytes)), media["size"])
}

func TestMCPUploadMediaBase64RejectsInvalidBase64(t *testing.T) {
	t.Parallel()

	srv := newMCPTestServer(t)
	out := srv.executeOperation(t, "web-token", mcpToolUploadBase64, map[string]any{
		"workspace_id":   "ws-1",
		"filename":       "flyer.png",
		"content_base64": "%%%not-valid-base64%%%",
	})
	require.Contains(t, out["error"].(map[string]any)["message"], "not valid base64")
	var count int
	require.NoError(t, srv.db.NewSelect().ColumnExpr("COUNT(*)").TableExpr("media_attachments").Scan(t.Context(), &count))
	require.Equal(t, 0, count)
}

func TestMCPUploadMediaBase64RejectsOversize(t *testing.T) {
	t.Parallel()

	srv := newMCPTestServer(t)
	_, rpcErr := srv.handler.uploadMediaBase64(t.Context(), "user-1", map[string]any{
		"workspace_id":   "ws-1",
		"filename":       "huge.png",
		"content_base64": base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, maxMCPBase64MediaBytes+1)),
	})
	require.NotNil(t, rpcErr)
	require.Contains(t, rpcErr.Message, "8 MiB")
	require.Contains(t, rpcErr.Message, "upload_media_from_url")
	var count int
	require.NoError(t, srv.db.NewSelect().ColumnExpr("COUNT(*)").TableExpr("media_attachments").Scan(t.Context(), &count))
	require.Equal(t, 0, count)
}

func TestMCPUploadMediaBase64RejectsUnsupportedMIME(t *testing.T) {
	t.Parallel()

	srv := newMCPTestServer(t)
	out := srv.executeOperation(t, "web-token", mcpToolUploadBase64, map[string]any{
		"workspace_id":   "ws-1",
		"filename":       "note.html",
		"mime_type":      "text/html",
		"content_base64": base64.StdEncoding.EncodeToString([]byte("<html><body>not media</body></html>")),
	})
	require.Contains(t, out["error"].(map[string]any)["message"], "unsupported mime type")
	var count int
	require.NoError(t, srv.db.NewSelect().ColumnExpr("COUNT(*)").TableExpr("media_attachments").Scan(t.Context(), &count))
	require.Equal(t, 0, count)
}

func TestMCPUploadMediaBase64RejectsViewerAndUnknownWorkspace(t *testing.T) {
	t.Parallel()

	srv := newMCPTestServer(t)
	_, err := srv.db.NewInsert().Model(&models.User{
		ID: "viewer-1", Email: "viewer@example.com", CreatedAt: time.Date(2026, 6, 30, 9, 30, 0, 0, time.UTC),
	}).Exec(t.Context())
	require.NoError(t, err)
	_, err = srv.db.NewInsert().Model(&models.WorkspaceMember{
		WorkspaceID: "ws-1", UserID: "viewer-1", Role: models.WorkspaceRoleViewer, Status: models.WorkspaceMemberStatusActive,
	}).Exec(t.Context())
	require.NoError(t, err)
	srv.handler.auth = mcpScopeAuthenticator{
		"web-token":    {UserID: "user-1", Email: "agent@example.com"},
		"viewer-token": {UserID: "viewer-1", Email: "viewer@example.com", Scope: "mcp:full"},
	}

	pngBytes := mcpTestPNG(t)
	payload := map[string]any{
		"workspace_id":   "ws-1",
		"filename":       "flyer.png",
		"content_base64": base64.StdEncoding.EncodeToString(pngBytes),
	}
	viewer := srv.executeOperation(t, "viewer-token", mcpToolUploadBase64, payload)
	require.Contains(t, viewer["error"].(map[string]any)["message"], "workspace editor role required")

	unknown := srv.executeOperation(t, "web-token", mcpToolUploadBase64, map[string]any{
		"workspace_id":   "ws-missing",
		"filename":       "flyer.png",
		"content_base64": payload["content_base64"],
	})
	require.Contains(t, unknown["error"].(map[string]any)["message"], "workspace not accessible")

	var count int
	require.NoError(t, srv.db.NewSelect().ColumnExpr("COUNT(*)").TableExpr("media_attachments").Scan(t.Context(), &count))
	require.Equal(t, 0, count)
}

func TestMCPListMediaRefreshesStalePublicURLFailure(t *testing.T) {
	t.Parallel()

	srv := newMCPTestServer(t)
	pngBytes := mcpTestPNG(t)
	calls := 0
	srv.setPublicMediaVerifier(t, publicurlVerifierFunc(func(_ context.Context, _ string) publicurl.Result {
		calls++
		if calls == 1 {
			return publicurl.Result{StatusCode: http.StatusNotFound, Error: "public media URL returned 404", CheckedAt: time.Now().UTC()}
		}
		return publicurl.Result{Ready: true, StatusCode: http.StatusOK, CheckedAt: time.Now().UTC()}
	}))

	created := srv.executeOperation(t, "web-token", mcpToolUploadBase64, map[string]any{
		"workspace_id":   "ws-1",
		"filename":       "flyer.png",
		"content_base64": base64.StdEncoding.EncodeToString(pngBytes),
	})
	require.NotContains(t, created, "error", created)
	mediaID := created["result"].(map[string]any)["structuredContent"].(map[string]any)["media"].(map[string]any)["id"].(string)

	var afterUpload models.MediaAttachment
	require.NoError(t, srv.db.NewSelect().Model(&afterUpload).Where("id = ?", mediaID).Scan(t.Context()))
	require.False(t, afterUpload.PublicURLReady)
	require.Equal(t, 0, afterUpload.PublicURLStatus)
	require.Empty(t, afterUpload.PublicURLError)

	_, err := srv.db.NewUpdate().
		Model((*models.MediaAttachment)(nil)).
		Set("public_url_checked_at = ?", time.Now().UTC().Add(-2*time.Minute)).
		Where("id = ?", mediaID).
		Exec(t.Context())
	require.NoError(t, err)

	listed := srv.queryOperation(t, "web-token", mcpToolListMedia, map[string]any{"workspace_id": "ws-1"})
	require.NotContains(t, listed, "error", listed)
	items := listed["result"].(map[string]any)["structuredContent"].(map[string]any)["media"].([]any)
	require.Len(t, items, 1)
	item := items[0].(map[string]any)
	require.Equal(t, mediaID, item["id"])
	require.Equal(t, true, item["public_url_ready"])
	require.Equal(t, float64(http.StatusOK), item["public_url_status"])

	var persisted models.MediaAttachment
	require.NoError(t, srv.db.NewSelect().Model(&persisted).Where("id = ?", mediaID).Scan(t.Context()))
	require.True(t, persisted.PublicURLReady)
	require.Equal(t, http.StatusOK, persisted.PublicURLStatus)
	require.GreaterOrEqual(t, calls, 2)
}

type publicurlVerifierFunc func(context.Context, string) publicurl.Result

func (f publicurlVerifierFunc) Verify(ctx context.Context, rawURL string) publicurl.Result {
	return f(ctx, rawURL)
}

func TestMCPUploadMediaBase64UnknownOperationNameIsGone(t *testing.T) {
	t.Parallel()

	srv := newMCPTestServer(t)
	out := srv.executeOperation(t, "web-token", "upload_media", map[string]any{
		"workspace_id": "ws-1",
	})
	require.Contains(t, out["error"].(map[string]any)["message"], "unknown operation")
}
