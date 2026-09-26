### Added

- MCP clients that only send text can upload a local file with `upload_media_base64` (workspace editor access, the same processing pipeline as other uploads, 8 MiB decoded / 12 MiB request body).

### Fixed

- Public media URL checks no longer stay stuck on an immediate 404. A first-check miss is stored as pending, failed checks refresh after one minute, and `list_media` plus publication validation re-check without a manual retry.
