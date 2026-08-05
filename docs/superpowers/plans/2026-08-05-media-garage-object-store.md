# Media → Garage Object Store (Sub-project B) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Store uploaded images in Garage (S3-compatible) behind the existing `media.Store` port, and stream them back through the app, selected by `APP_MEDIA_DRIVER`.

**Architecture:** Add a `Store.Open(key)` read method and a shared `processImage` helper; add an `S3Store` adapter (aws-sdk-go-v2, path-style) alongside `LocalStore`; serve `GET /media/*key` by streaming from whichever adapter is wired. The compose stack gains an internal Garage node, a one-shot `garage-init`, and an `rclone` mirror sidecar; Caddy proxies `/media/*` to the api.

**Tech Stack:** Go 1.26, gin, aws-sdk-go-v2 (`config`, `credentials`, `service/s3`), Garage (`dxflrs/garage`), rclone, Docker Compose, Caddy.

## Global Constraints

- TDD mandatory: failing test → confirm fail → minimal code → confirm pass → refactor.
- No change to image processing rules: decode (JPEG/PNG/WebP only) → `imaging.Fit` longest side 1600px → JPEG quality 80 → key `<sha256[:2]>/<sha256>.jpg` (forward slashes).
- No frontend change; image URLs stay `/media/<key>`.
- No route-string changes except swapping the `/media` serving mechanism.
- Existing test suite (226 Go tests) stays green after every task. Run `rtk go test ./...`. Build: `CGO_ENABLED=0 go build ./...`. Vet: `go vet ./...`.
- Never commit to `main`; work on branch `feat/media-garage-object-store` (already created).
- The `media.Store` port methods stay context-free; `S3Store` uses `context.Background()` internally.
- Garage has NO published port; only the api holds S3 credentials.
- Uber Go style; American English; organized imports (`goimports`; if the binary is missing, `go run golang.org/x/tools/cmd/goimports@latest -w <files>`).
- Builders do NOT run git or touch `CONTEXT.md`/docker state — the orchestrator commits after review.

---

## File Structure

- `internal/media/store.go` — add `Object`, `ErrNotFound`, `Open` on the `Store` port + `LocalStore.Open`; extract `processImage`; refactor `LocalStore.SaveReader` to use it.
- `internal/media/s3store.go` — **new**: `s3API` interface, `S3Store`, `NewS3Store`, its `SaveReader`/`SaveMultipart`/`UsageBytes`/`Open`.
- `internal/media/s3store_test.go` — **new**: fake-`s3API` unit tests + skipped real-Garage integration test.
- `internal/media/store_test.go` — add `Open` + `processImage` tests.
- `internal/router/media.go` — **new**: `mediaHandler(store media.Store) gin.HandlerFunc`.
- `internal/router/router.go` — swap `engine.Static("/media", …)` for `engine.GET("/media/*key", mediaHandler(deps.Media))`.
- `internal/router/router_test.go` — add a fake-store handler test (existing media tests keep passing unchanged).
- `internal/config/config.go` — media driver + S3 fields.
- `internal/config/config_test.go` — defaults/predicate tests.
- `cmd/server/main.go` — build the store from the driver.
- `go.mod` / `go.sum` — aws-sdk-go-v2 modules.
- `deploy/garage/garage.toml`, `deploy/garage/init.sh` — **new** Garage config + bootstrap.
- `deploy/backup/media-backup.sh` — **new** rclone mirror loop.
- `deploy/caddy/Caddyfile` — `/media/*` → `reverse_proxy api:8080`.
- `docker-compose.yaml`, `docker-compose.dev.yaml`, `.env.example` — Garage + init + mirror wiring.
- `docs/adr/0004-media-object-storage-garage.md` — **new** ADR.

---

### Task 1: `media.Store` gains `Open`, `ErrNotFound`, and a shared `processImage` helper

**Files:**
- Modify: `internal/media/store.go`
- Test: `internal/media/store_test.go`

**Interfaces:**
- Produces:
  - `var media.ErrNotFound error`
  - `type media.Object struct { Body io.ReadCloser; ContentType string; Size int64 }`
  - `Open(key string) (Object, error)` added to the `Store` interface and implemented on `*LocalStore`.
  - unexported `processImage(r io.Reader) (key string, jpeg []byte, err error)`.

- [ ] **Step 1: Write the failing tests** — append to `internal/media/store_test.go`:

```go
func TestLocalStoreOpenRoundTrip(t *testing.T) {
	s := media.NewLocalStore(t.TempDir())
	// A 2x2 PNG is a valid decodable image; SaveReader re-encodes to JPEG.
	key, err := s.SaveReader(bytes.NewReader(tinyPNG(t)))
	if err != nil {
		t.Fatalf("SaveReader: %v", err)
	}
	obj, err := s.Open(key)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer obj.Body.Close()
	if obj.ContentType != "image/jpeg" {
		t.Fatalf("ContentType = %q, want image/jpeg", obj.ContentType)
	}
	b, _ := io.ReadAll(obj.Body)
	if int64(len(b)) != obj.Size || obj.Size == 0 {
		t.Fatalf("Size = %d, read %d bytes", obj.Size, len(b))
	}
}

func TestLocalStoreOpenMissingIsErrNotFound(t *testing.T) {
	s := media.NewLocalStore(t.TempDir())
	if _, err := s.Open("ab/does-not-exist.jpg"); !errors.Is(err, media.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}
```

Add a `tinyPNG(t *testing.T) []byte` helper if one is not already present (encode a 2x2 `image.NewRGBA` via `png.Encode` into a buffer). Ensure imports `bytes`, `errors`, `io`, `image`, `image/png`.

- [ ] **Step 2: Run — expect FAIL**

Run: `rtk go test ./internal/media/`
Expected: FAIL — `Open` undefined, `ErrNotFound` undefined.

- [ ] **Step 3: Implement.** In `store.go`: add `"errors"` to imports; add the sentinel, `Object`, extend the interface, add `LocalStore.Open`, extract `processImage`, and refactor `SaveReader`:

```go
// ErrNotFound reports that no stored object has the requested key.
var ErrNotFound = errors.New("media: object not found")

// Object is a readable stored image plus the metadata the serving handler needs.
type Object struct {
	Body        io.ReadCloser
	ContentType string
	Size        int64
}

// Store saves, serves, and measures uploaded images. LocalStore is the
// filesystem adapter; S3Store is the Garage adapter.
type Store interface {
	SaveReader(r io.Reader) (string, error)
	SaveMultipart(fh *multipart.FileHeader) (string, error)
	UsageBytes() (int64, error)
	Open(key string) (Object, error)
}

// processImage validates, downscales, and JPEG-encodes r, returning the
// content-hash key (forward-slashed) and the encoded bytes.
func processImage(r io.Reader) (key string, jpeg []byte, err error) {
	br := bufio.NewReader(r)
	head, _ := br.Peek(512)
	switch ct := http.DetectContentType(head); ct {
	case "image/jpeg", "image/png", "image/webp":
	default:
		return "", nil, fmt.Errorf("media: unsupported image type %q (want JPEG, PNG, or WebP)", ct)
	}
	img, err := imaging.Decode(br)
	if err != nil {
		return "", nil, fmt.Errorf("media: decode image: %w", err)
	}
	resized := imaging.Fit(img, maxDimension, maxDimension, imaging.Lanczos)
	var buf bytes.Buffer
	if err := imaging.Encode(&buf, resized, imaging.JPEG, imaging.JPEGQuality(jpegQuality)); err != nil {
		return "", nil, fmt.Errorf("media: encode image: %w", err)
	}
	data := buf.Bytes()
	sum := sha256.Sum256(data)
	hexSum := hex.EncodeToString(sum[:])
	return path.Join(hexSum[:2], hexSum+".jpg"), data, nil
}
```

Refactor `SaveReader` to delegate:

```go
func (s *LocalStore) SaveReader(r io.Reader) (string, error) {
	key, data, err := processImage(r)
	if err != nil {
		return "", err
	}
	full := filepath.Join(s.Dir, filepath.FromSlash(key))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return "", fmt.Errorf("media: create dir: %w", err)
	}
	if err := os.WriteFile(full, data, 0o644); err != nil {
		return "", fmt.Errorf("media: write file: %w", err)
	}
	return key, nil
}

// Open returns the stored object for key, or ErrNotFound.
func (s *LocalStore) Open(key string) (Object, error) {
	f, err := os.Open(filepath.Join(s.Dir, filepath.FromSlash(key)))
	if err != nil {
		if os.IsNotExist(err) {
			return Object{}, ErrNotFound
		}
		return Object{}, err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return Object{}, err
	}
	return Object{Body: f, ContentType: "image/jpeg", Size: info.Size()}, nil
}
```

Add `"path"` to imports (used by `processImage`). Keep `"path/filepath"`.

- [ ] **Step 4: Run — expect PASS**

Run: `rtk go test ./internal/media/` then `rtk go test ./...`
Expected: PASS (existing media/store tests still green; `SaveReader` behavior unchanged — same keys, same bytes).

- [ ] **Step 5: Commit** — orchestrator commits: `refactor(media): add Store.Open + ErrNotFound; extract processImage`.

---

### Task 2: Stream `/media/*key` through the port

**Files:**
- Create: `internal/router/media.go`
- Modify: `internal/router/router.go` (lines ~99-104)
- Test: `internal/router/router_test.go`

**Interfaces:**
- Consumes: `media.Store.Open`, `media.ErrNotFound`, `media.Object` (Task 1).
- Produces: `mediaHandler(store media.Store) gin.HandlerFunc`.

- [ ] **Step 1: Write the failing test** — append to `router_test.go`. It uses a fake store so it asserts the new headers and 404 mapping directly:

```go
type fakeStore struct{ obj media.Object; err error }

func (f fakeStore) SaveReader(io.Reader) (string, error)            { return "", nil }
func (f fakeStore) SaveMultipart(*multipart.FileHeader) (string, error) { return "", nil }
func (f fakeStore) UsageBytes() (int64, error)                      { return 0, nil }
func (f fakeStore) Open(string) (media.Object, error)              { return f.obj, f.err }

func TestMediaRouteSetsImmutableCacheHeader(t *testing.T) {
	mediaDir := t.TempDir()
	deps := newDeps(t, mediaDir)
	deps.Media = fakeStore{obj: media.Object{
		Body: io.NopCloser(strings.NewReader("jpegbytes")), ContentType: "image/jpeg", Size: 9,
	}}
	engine := router.NewEngine(deps)

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/media/ab/x.jpg", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Fatalf("Cache-Control = %q", got)
	}
	if rec.Header().Get("Content-Type") != "image/jpeg" {
		t.Fatalf("Content-Type = %q", rec.Header().Get("Content-Type"))
	}
}

func TestMediaRouteNotFoundMapsTo404(t *testing.T) {
	deps := newDeps(t, t.TempDir())
	deps.Media = fakeStore{err: media.ErrNotFound}
	engine := router.NewEngine(deps)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/media/ab/x.jpg", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}
```

Add imports to the test file: `io`, `mime/multipart`, `strings`.

- [ ] **Step 2: Run — expect FAIL**

Run: `rtk go test ./internal/router/`
Expected: FAIL — new handler not wired; `/media/ab/x.jpg` currently hits `engine.Static` on an empty temp dir → 404, so the cache-header test fails on the header assertion.

- [ ] **Step 3: Implement.** Create `internal/router/media.go`:

```go
package router

import (
	"errors"
	"io"
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"phum-panya/internal/media"
)

// mediaHandler streams a stored image at /media/<key> from the media store.
// Keys are content hashes, so responses are immutable and cacheable forever.
func mediaHandler(store media.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := strings.TrimPrefix(c.Param("key"), "/")
		if key == "" || strings.Contains(key, "..") || path.Clean(key) != key {
			c.Status(http.StatusNotFound)
			return
		}
		obj, err := store.Open(key)
		if err != nil {
			if errors.Is(err, media.ErrNotFound) {
				c.Status(http.StatusNotFound)
				return
			}
			c.Status(http.StatusInternalServerError)
			return
		}
		defer obj.Body.Close()
		h := c.Writer.Header()
		h.Set("Content-Type", obj.ContentType)
		if obj.Size > 0 {
			h.Set("Content-Length", strconv.FormatInt(obj.Size, 10))
		}
		h.Set("Cache-Control", "public, max-age=31536000, immutable")
		c.Status(http.StatusOK)
		_, _ = io.Copy(c.Writer, obj.Body)
	}
}
```

In `router.go`, replace the media block (keep the guarded `MkdirAll` so a local-driver dir still exists and the existing missing-dir test stays valid):

```go
	// A local-driver media dir may not exist yet; create it so the local
	// adapter can serve/write. Harmless (and skipped) when MediaDir is empty
	// (s3 driver). Serving no longer depends on the dir — Open maps a missing
	// file to a 404.
	if deps.Cfg.MediaDir != "" {
		_ = os.MkdirAll(deps.Cfg.MediaDir, 0o755)
	}
	// Public, no-auth; registered before webui.Register so its SPA catch-all
	// never shadows a photo. The handler streams from whichever media adapter
	// is wired (LocalStore or S3Store/Garage).
	engine.GET("/media/*key", mediaHandler(deps.Media))
```

- [ ] **Step 4: Run — expect PASS**

Run: `rtk go test ./internal/router/ ./...`
Expected: PASS. The three pre-existing media tests still hold — `TestMediaRouteServesStoredFile` (LocalStore streams the file), `TestMediaRouteMissingMediaDirDoesNotServe` (dir still created by the guarded `MkdirAll`; missing file → 404, not 500), `TestMediaRouteTraversalBlocked` (`../secret.txt` key contains `..` → 404).

- [ ] **Step 5: Commit** — `feat(media): stream /media/<key> through the Store port`.

---

### Task 3: Config — media driver + S3 settings

**Files:**
- Modify: `internal/config/config.go`
- Test: `internal/config/config_test.go`

**Interfaces:**
- Produces: `Config` fields `MediaDriver, S3Endpoint, S3Region, S3Bucket, S3AccessKey, S3SecretKey string` and `S3UsePathStyle bool`; predicate `UsesS3Media() bool`.

- [ ] **Step 1: Write the failing test** — append to `config_test.go`:

```go
func TestMediaDriverDefaultsToLocal(t *testing.T) {
	t.Setenv("APP_MEDIA_DRIVER", "")
	c := config.Load()
	if c.MediaDriver != "local" || c.UsesS3Media() {
		t.Fatalf("default MediaDriver = %q, UsesS3Media = %v; want local/false", c.MediaDriver, c.UsesS3Media())
	}
}

func TestS3ConfigLoadedWhenDriverS3(t *testing.T) {
	t.Setenv("APP_MEDIA_DRIVER", "s3")
	t.Setenv("APP_S3_ENDPOINT", "http://garage:3900")
	t.Setenv("APP_S3_BUCKET", "media")
	c := config.Load()
	if !c.UsesS3Media() {
		t.Fatalf("UsesS3Media = false, want true")
	}
	if c.S3Endpoint != "http://garage:3900" || c.S3Bucket != "media" || c.S3Region != "garage" || !c.S3UsePathStyle {
		t.Fatalf("s3 config not loaded: %+v", c)
	}
}
```

- [ ] **Step 2: Run — expect FAIL** (`rtk go test ./internal/config/`) — fields/predicate undefined.

- [ ] **Step 3: Implement.** Add to the `Config` struct and to `Load()`:

```go
	MediaDriver    string
	S3Endpoint     string
	S3Region       string
	S3Bucket       string
	S3AccessKey    string
	S3SecretKey    string
	S3UsePathStyle bool
```

```go
	MediaDriver:    env("APP_MEDIA_DRIVER", "local"),
	S3Endpoint:     env("APP_S3_ENDPOINT", ""),
	S3Region:       env("APP_S3_REGION", "garage"),
	S3Bucket:       env("APP_S3_BUCKET", "media"),
	S3AccessKey:    env("APP_S3_ACCESS_KEY", ""),
	S3SecretKey:    env("APP_S3_SECRET_KEY", ""),
	S3UsePathStyle: env("APP_S3_USE_PATH_STYLE", "true") != "false",
```

```go
// UsesS3Media reports whether uploaded media is stored in the S3/Garage
// backend rather than the local filesystem.
func (c Config) UsesS3Media() bool { return c.MediaDriver == "s3" }
```

- [ ] **Step 4: Run — expect PASS** (`rtk go test ./internal/config/ ./...`).

- [ ] **Step 5: Commit** — `feat(config): media driver + S3 settings`.

---

### Task 4: `S3Store` Garage adapter (aws-sdk-go-v2, fake-testable)

**Files:**
- Modify: `go.mod`, `go.sum`
- Create: `internal/media/s3store.go`, `internal/media/s3store_test.go`

**Interfaces:**
- Consumes: `processImage`, `Object`, `ErrNotFound` (Task 1).
- Produces: `NewS3Store(ctx context.Context, endpoint, region, accessKey, secretKey, bucket string, usePathStyle bool) (*S3Store, error)`; `*S3Store` satisfies `media.Store`.

- [ ] **Step 1: Add the dependency**

Run:
```bash
go get github.com/aws/aws-sdk-go-v2/config@latest
go get github.com/aws/aws-sdk-go-v2/credentials@latest
go get github.com/aws/aws-sdk-go-v2/service/s3@latest
go mod tidy
```
(Needs network access to the Go module proxy.)

- [ ] **Step 2: Write the failing tests** — `internal/media/s3store_test.go` (internal `package media` so it can use the unexported `s3API` seam):

```go
package media

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type fakeS3 struct {
	put    map[string][]byte
	getErr error
}

func (f *fakeS3) PutObject(_ context.Context, in *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	b, _ := io.ReadAll(in.Body)
	if f.put == nil {
		f.put = map[string][]byte{}
	}
	f.put[*in.Key] = b
	return &s3.PutObjectOutput{}, nil
}

func (f *fakeS3) GetObject(_ context.Context, in *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	b, ok := f.put[*in.Key]
	if !ok {
		return nil, &types.NoSuchKey{}
	}
	return &s3.GetObjectOutput{
		Body:          io.NopCloser(bytes.NewReader(b)),
		ContentType:   aws.String("image/jpeg"),
		ContentLength: aws.Int64(int64(len(b))),
	}, nil
}

func (f *fakeS3) ListObjectsV2(_ context.Context, _ *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
	var out s3.ListObjectsV2Output
	for k, b := range f.put {
		out.Contents = append(out.Contents, types.Object{Key: aws.String(k), Size: aws.Int64(int64(len(b)))})
	}
	return &out, nil
}

func TestS3StoreSaveThenOpen(t *testing.T) {
	f := &fakeS3{}
	s := &S3Store{api: f, bucket: "media"}
	key, err := s.SaveReader(bytes.NewReader(tinyPNG(t)))
	if err != nil {
		t.Fatalf("SaveReader: %v", err)
	}
	if _, ok := f.put[key]; !ok {
		t.Fatalf("object not put under key %q", key)
	}
	obj, err := s.Open(key)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer obj.Body.Close()
	if obj.ContentType != "image/jpeg" || obj.Size == 0 {
		t.Fatalf("bad object meta: %+v", obj)
	}
}

func TestS3StoreOpenMissingIsErrNotFound(t *testing.T) {
	s := &S3Store{api: &fakeS3{}, bucket: "media"}
	if _, err := s.Open("ab/missing.jpg"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestS3StoreUsageBytesSums(t *testing.T) {
	f := &fakeS3{}
	s := &S3Store{api: f, bucket: "media"}
	if _, err := s.SaveReader(bytes.NewReader(tinyPNG(t))); err != nil {
		t.Fatalf("SaveReader: %v", err)
	}
	n, err := s.UsageBytes()
	if err != nil || n == 0 {
		t.Fatalf("UsageBytes = %d, err %v", n, err)
	}
}
```

(`tinyPNG` is the helper added in Task 1's `store_test.go`; if that file is `package media_test` and this one is `package media`, add a small local `tinyPNG` here instead — a 2x2 `png.Encode` into a buffer.)

- [ ] **Step 3: Run — expect FAIL** (`rtk go test ./internal/media/`) — `S3Store`, `s3API` undefined.

- [ ] **Step 4: Implement** `internal/media/s3store.go`:

```go
package media

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// s3API is the subset of the S3 client S3Store uses; a fake implements it in tests.
type s3API interface {
	PutObject(context.Context, *s3.PutObjectInput, ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	GetObject(context.Context, *s3.GetObjectInput, ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	ListObjectsV2(context.Context, *s3.ListObjectsV2Input, ...func(*s3.Options)) (*s3.ListObjectsV2Output, error)
}

// S3Store stores images in an S3-compatible bucket (Garage).
type S3Store struct {
	api    s3API
	bucket string
}

// NewS3Store builds an S3Store with static credentials and a fixed endpoint.
func NewS3Store(ctx context.Context, endpoint, region, accessKey, secretKey, bucket string, usePathStyle bool) (*S3Store, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
	)
	if err != nil {
		return nil, fmt.Errorf("media: aws config: %w", err)
	}
	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.UsePathStyle = usePathStyle
	})
	return &S3Store{api: client, bucket: bucket}, nil
}

func (s *S3Store) SaveReader(r io.Reader) (string, error) {
	key, data, err := processImage(r)
	if err != nil {
		return "", err
	}
	_, err = s.api.PutObject(context.Background(), &s3.PutObjectInput{
		Bucket:        aws.String(s.bucket),
		Key:           aws.String(key),
		Body:          bytes.NewReader(data),
		ContentType:   aws.String("image/jpeg"),
		ContentLength: aws.Int64(int64(len(data))),
	})
	if err != nil {
		return "", fmt.Errorf("media: put object: %w", err)
	}
	return key, nil
}

func (s *S3Store) SaveMultipart(fh *multipart.FileHeader) (string, error) {
	f, err := fh.Open()
	if err != nil {
		return "", fmt.Errorf("media: open upload: %w", err)
	}
	defer f.Close()
	return s.SaveReader(f)
}

func (s *S3Store) UsageBytes() (int64, error) {
	var total int64
	var token *string
	for {
		out, err := s.api.ListObjectsV2(context.Background(), &s3.ListObjectsV2Input{
			Bucket:            aws.String(s.bucket),
			ContinuationToken: token,
		})
		if err != nil {
			return 0, fmt.Errorf("media: list objects: %w", err)
		}
		for _, o := range out.Contents {
			total += aws.ToInt64(o.Size)
		}
		if out.IsTruncated == nil || !*out.IsTruncated {
			return total, nil
		}
		token = out.NextContinuationToken
	}
}

func (s *S3Store) Open(key string) (Object, error) {
	out, err := s.api.GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		var nsk *types.NoSuchKey
		var nf *types.NotFound
		if errors.As(err, &nsk) || errors.As(err, &nf) {
			return Object{}, ErrNotFound
		}
		return Object{}, fmt.Errorf("media: get object: %w", err)
	}
	return Object{Body: out.Body, ContentType: aws.ToString(out.ContentType), Size: aws.ToInt64(out.ContentLength)}, nil
}
```

Add a skipped real-Garage integration test to `s3store_test.go`:

```go
func TestS3StoreIntegration(t *testing.T) {
	endpoint := os.Getenv("APP_S3_ENDPOINT")
	if endpoint == "" {
		t.Skip("set APP_S3_ENDPOINT to run the real-Garage integration test")
	}
	// build via NewS3Store from APP_S3_* env and round-trip a tinyPNG.
}
```

- [ ] **Step 5: Run — expect PASS** (`rtk go test ./internal/media/ ./...`) and `CGO_ENABLED=0 go build ./...`.

- [ ] **Step 6: Commit** — `feat(media): add S3Store Garage adapter`.

---

### Task 5: Wire driver selection in `main.go`

**Files:**
- Modify: `cmd/server/main.go`
- Test: `cmd/server/main_smoke_test.go` (add a small selection unit test)

**Interfaces:**
- Consumes: `media.NewLocalStore`, `media.NewS3Store`, `config.Config`.
- Produces: `newMediaStore(ctx context.Context, cfg config.Config) (media.Store, error)`.

- [ ] **Step 1: Write the failing test** — append to `cmd/server/main_smoke_test.go`:

```go
func TestNewMediaStoreLocal(t *testing.T) {
	s, err := newMediaStore(context.Background(), config.Config{MediaDriver: "local", MediaDir: t.TempDir()})
	if err != nil {
		t.Fatalf("newMediaStore: %v", err)
	}
	if _, ok := s.(*media.LocalStore); !ok {
		t.Fatalf("got %T, want *media.LocalStore", s)
	}
}
```

Ensure the test file imports `context`, `config`, `media`.

- [ ] **Step 2: Run — expect FAIL** (`rtk go test ./cmd/server/`) — `newMediaStore` undefined.

- [ ] **Step 3: Implement** in `main.go`:

```go
// newMediaStore builds the media adapter selected by cfg.MediaDriver.
func newMediaStore(ctx context.Context, cfg config.Config) (media.Store, error) {
	if cfg.UsesS3Media() {
		return media.NewS3Store(ctx, cfg.S3Endpoint, cfg.S3Region, cfg.S3AccessKey, cfg.S3SecretKey, cfg.S3Bucket, cfg.S3UsePathStyle)
	}
	return media.NewLocalStore(cfg.MediaDir), nil
}
```

In `runServer`, replace `Media: media.NewLocalStore(cfg.MediaDir),` with a built value:

```go
	mediaStore, err := newMediaStore(context.Background(), cfg)
	if err != nil {
		log.Fatalf("media store: %v", err)
	}
	// ... deps := router.Deps{ ... Media: mediaStore, ... }
```

- [ ] **Step 4: Run — expect PASS** (`rtk go test ./... ` and `CGO_ENABLED=0 go build ./...`).

- [ ] **Step 5: Commit** — `feat(server): select media backend from APP_MEDIA_DRIVER`.

---

### Task 6: Garage service + config + idempotent init (prod compose)

**Files:**
- Create: `deploy/garage/garage.toml`, `deploy/garage/init.sh`
- Modify: `docker-compose.yaml`

- [ ] **Step 1: Write `deploy/garage/garage.toml`** (single node, internal):

```toml
metadata_dir = "/var/lib/garage/meta"
data_dir = "/var/lib/garage/data"
db_engine = "sqlite"
replication_factor = 1
rpc_bind_addr = "[::]:3901"
rpc_public_addr = "127.0.0.1:3901"
rpc_secret = "$GARAGE_RPC_SECRET"

[s3_api]
s3_region = "garage"
api_bind_addr = "[::]:3900"
root_domain = ".s3.garage"

[s3_web]
bind_addr = "[::]:3902"
root_domain = ".web.garage"
index = "index.html"
```

- [ ] **Step 2: Write `deploy/garage/init.sh`** — idempotent bootstrap (layout → bucket → import fixed key → grant):

```sh
#!/bin/sh
set -eu
# Wait for the Garage RPC to answer.
until garage status >/dev/null 2>&1; do sleep 1; done

# Assign + apply a layout once (idempotent: skip if a role is already set).
if ! garage layout show | grep -q "Zone"; then
  NODE_ID=$(garage node id -q | cut -d@ -f1)
  garage layout assign -z dc1 -c 1G "$NODE_ID"
  garage layout apply --version 1
fi

# Bucket (ignore "already exists").
garage bucket create "$APP_S3_BUCKET" 2>/dev/null || true

# Import the fixed key so the api's env credentials match (idempotent).
if ! garage key info "$APP_S3_ACCESS_KEY" >/dev/null 2>&1; then
  garage key import --yes "$APP_S3_ACCESS_KEY" "$APP_S3_SECRET_KEY" -n phumpanya
fi

garage bucket allow --read --write --key "$APP_S3_ACCESS_KEY" "$APP_S3_BUCKET"
echo "garage-init: done"
```

Note for the implementer: `garage key import` flag/arg order varies by Garage version. Verify against the pinned image's `garage key import --help` and adjust; the intent is "register a key with THIS access-key-id + secret", not "generate a random one".

- [ ] **Step 3: Add services to `docker-compose.yaml`** (garage + one-shot init), and the volumes:

```yaml
  garage:
    image: dxflrs/garage:v1.0.1
    restart: unless-stopped
    environment:
      GARAGE_RPC_SECRET: ${GARAGE_RPC_SECRET:?set GARAGE_RPC_SECRET}
    volumes:
      - garage-meta:/var/lib/garage/meta
      - garage-data:/var/lib/garage/data
      - ./deploy/garage/garage.toml:/etc/garage.toml:ro
    healthcheck:
      test: ["CMD", "/garage", "status"]
      interval: 10s
      timeout: 5s
      retries: 5

  garage-init:
    image: dxflrs/garage:v1.0.1
    depends_on:
      garage:
        condition: service_healthy
    environment:
      GARAGE_RPC_SECRET: ${GARAGE_RPC_SECRET:?set GARAGE_RPC_SECRET}
      APP_S3_BUCKET: ${APP_S3_BUCKET:-media}
      APP_S3_ACCESS_KEY: ${APP_S3_ACCESS_KEY:?set APP_S3_ACCESS_KEY}
      APP_S3_SECRET_KEY: ${APP_S3_SECRET_KEY:?set APP_S3_SECRET_KEY}
    entrypoint: ["/bin/sh", "/init.sh"]
    volumes:
      - ./deploy/garage/garage.toml:/etc/garage.toml:ro
      - ./deploy/garage/init.sh:/init.sh:ro
    restart: on-failure
```

Add to the top-level `volumes:` block: `garage-meta:` and `garage-data:`.

Note: `garage-init` needs the same `garage.toml` (so the CLI finds the RPC socket/secret); it shares the config and RPC secret with the node.

- [ ] **Step 4: Verify**

Run: `docker compose -f docker-compose.yaml config -q && echo COMPOSE_OK`
Expected: no error (interpolation + schema valid). Full `docker compose up` bring-up (garage healthy, `garage-init` exits 0, bucket+key created) is the orchestrator's integration check — note it in the report; it needs Docker + the `.env` secrets.

- [ ] **Step 5: Commit** — `feat(deploy): add internal Garage node + idempotent init`.

---

### Task 7: Media mirror sidecar + api env + Caddy serving swap

**Files:**
- Create: `deploy/backup/media-backup.sh`
- Modify: `docker-compose.yaml` (api env + media-backup service + volumes; drop shared `media` volume), `deploy/caddy/Caddyfile`

- [ ] **Step 1: Write `deploy/backup/media-backup.sh`** — periodic rclone mirror:

```sh
#!/bin/sh
set -eu
export RCLONE_CONFIG_GARAGE_TYPE=s3
export RCLONE_CONFIG_GARAGE_PROVIDER=Other
export RCLONE_CONFIG_GARAGE_ENDPOINT="$APP_S3_ENDPOINT"
export RCLONE_CONFIG_GARAGE_ACCESS_KEY_ID="$APP_S3_ACCESS_KEY"
export RCLONE_CONFIG_GARAGE_SECRET_ACCESS_KEY="$APP_S3_SECRET_KEY"
export RCLONE_CONFIG_GARAGE_FORCE_PATH_STYLE=true

INTERVAL="${MEDIA_BACKUP_INTERVAL:-86400}"
while true; do
  echo "media-backup: syncing garage:$APP_S3_BUCKET -> /backups/media"
  rclone sync "garage:$APP_S3_BUCKET" /backups/media || echo "media-backup: sync failed"
  sleep "$INTERVAL"
done
```

- [ ] **Step 2: Add the `media-backup` service and api env to `docker-compose.yaml`.** Add to the `api` service `environment:` block:

```yaml
      APP_MEDIA_DRIVER: s3
      APP_S3_ENDPOINT: http://garage:3900
      APP_S3_REGION: garage
      APP_S3_BUCKET: ${APP_S3_BUCKET:-media}
      APP_S3_ACCESS_KEY: ${APP_S3_ACCESS_KEY:?set APP_S3_ACCESS_KEY}
      APP_S3_SECRET_KEY: ${APP_S3_SECRET_KEY:?set APP_S3_SECRET_KEY}
```

Add to the api `depends_on:`:

```yaml
      garage:
        condition: service_healthy
```

Remove the `media:/data/media` volume from `api` and `media:/srv/media:ro` from `caddy` (bytes now live in Garage; the api streams them). Remove `media` from the top-level `volumes:` block. Add the sidecar:

```yaml
  media-backup:
    image: rclone/rclone:latest
    restart: unless-stopped
    depends_on:
      garage:
        condition: service_healthy
    entrypoint: ["/bin/sh", "/media-backup.sh"]
    environment:
      APP_S3_ENDPOINT: http://garage:3900
      APP_S3_BUCKET: ${APP_S3_BUCKET:-media}
      APP_S3_ACCESS_KEY: ${APP_S3_ACCESS_KEY:?set APP_S3_ACCESS_KEY}
      APP_S3_SECRET_KEY: ${APP_S3_SECRET_KEY:?set APP_S3_SECRET_KEY}
    volumes:
      - ./deploy/backup/media-backup.sh:/media-backup.sh:ro
      - media-backups:/backups
```

Add `media-backups:` to the top-level `volumes:` block.

- [ ] **Step 3: Swap Caddy `/media` to the api.** In `deploy/caddy/Caddyfile`, replace the `handle /media/*` block:

```caddyfile
	# Media reads are streamed by the Go service from object storage; Caddy
	# proxies them like the rest of the API.
	handle /media/* {
		reverse_proxy api:8080
	}
```

- [ ] **Step 4: Verify**

Run: `docker compose -f docker-compose.yaml config -q && echo COMPOSE_OK`
Expected: valid. Confirm `grep -n "srv/media\|media:/data/media\|^  media:" docker-compose.yaml` returns nothing (shared media volume fully removed). Real bring-up + an upload→GET round-trip is the orchestrator's integration check.

- [ ] **Step 5: Commit** — `feat(deploy): stream media from Garage via api; add mirror sidecar`.

---

### Task 8: Dev-compose parity, `.env.example`, ADR

**Files:**
- Modify: `docker-compose.dev.yaml`, `.env.example`
- Create: `docs/adr/0004-media-object-storage-garage.md`

- [ ] **Step 1: Dev parity.** Add the same `garage` + `garage-init` services to `docker-compose.dev.yaml` (reuse `deploy/garage/garage.toml` and `deploy/garage/init.sh`), and add the `APP_MEDIA_DRIVER=s3` + `APP_S3_*` env to the dev `api` service, with `depends_on: garage: {condition: service_healthy}`. Use the same `GARAGE_RPC_SECRET`/`APP_S3_*` env vars (dev `.env` defaults may be weak, matching the existing `POSTGRES_PASSWORD` dev-default pattern). A dev media-backup sidecar is NOT required.

- [ ] **Step 2: `.env.example`.** Add a documented block:

```sh
# --- Media object storage (prod + dev stacks: Garage) ----------------------
# The api stores uploads in an internal Garage bucket over the S3 API; Caddy
# proxies /media/* to the api, which streams the bytes. Garage has no public
# port. garage-init imports THIS exact key so the api can authenticate.
GARAGE_RPC_SECRET=change-me-to-a-64-hex-char-secret
APP_S3_ACCESS_KEY=change-me-access-key
APP_S3_SECRET_KEY=change-me-to-a-long-random-secret
APP_S3_BUCKET=media
```

- [ ] **Step 3: ADR.** Write `docs/adr/0004-media-object-storage-garage.md` following the format of `docs/adr/0003-optional-compose-stack-postgres-caddy.md`: context (media must leave local disk for 15-Factor statelessness), decision (Garage behind the `media.Store` port, selected by `APP_MEDIA_DRIVER`; app streams; mirror sidecar for durability), consequences (single-node caveat; new aws-sdk dependency; dev/prod parity).

- [ ] **Step 4: Verify**

Run: `docker compose -f docker-compose.dev.yaml config -q && docker compose -f docker-compose.yaml config -q && echo BOTH_OK`
Expected: both valid.

- [ ] **Step 5: Commit** — `feat(deploy): dev Garage parity; document media object storage (ADR-0004)`.

- [ ] **Step 6 (orchestrator):** update `CONTEXT.md` with the sub-project B entry and run the final whole-branch review before integration.

---

## Self-Review

**Spec coverage:**
- Port `Open` + `ErrNotFound` + shared `processImage` → Task 1.
- Streaming handler + immutable cache header + traversal guard + Caddy swap → Task 2 (handler) + Task 7 (Caddy).
- Config driver + S3 settings → Task 3.
- `S3Store` (aws-sdk-go-v2, fake-testable, path-style, UsageBytes pagination, NoSuchKey→ErrNotFound) → Task 4.
- Driver selection in main → Task 5.
- Garage service + idempotent init (fixed-key import) → Task 6.
- Mirror sidecar (durability) + api env + drop media volume → Task 7.
- Dev parity + `.env.example` + ADR-0004 → Task 8.
- No frontend change, no route change beyond `/media` mechanism — held across all tasks.

**Placeholder scan:** The two ops notes ("verify `garage key import` arg order against the pinned image", "real `docker compose up` is the orchestrator's integration check") are deliberate verification instructions, not placeholders — the version-specific CLI surface and Docker runtime genuinely cannot be pinned from here, and every code/YAML block is complete.

**Type consistency:** `media.Store` interface gains exactly `Open(key string) (Object, error)`; `Object{Body,ContentType,Size}` and `ErrNotFound` are used identically in Tasks 1, 2, 4. `s3API`'s three methods match the aws-sdk-go-v2 signatures used by `S3Store` and the `fakeS3` test double. `newMediaStore(ctx, cfg)` (Task 5) consumes `NewS3Store` (Task 4) and `NewLocalStore` with matching signatures. Env var names (`APP_MEDIA_DRIVER`, `APP_S3_ENDPOINT`, `APP_S3_REGION`, `APP_S3_BUCKET`, `APP_S3_ACCESS_KEY`, `APP_S3_SECRET_KEY`, `APP_S3_USE_PATH_STYLE`, `GARAGE_RPC_SECRET`) are identical across config (Task 3), compose (Tasks 6-8), and `.env.example`.
