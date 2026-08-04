package media_test

import (
	"bytes"
	"crypto/rand"
	stdimage "image"
	"image/jpeg"
	"image/png"
	"mime/multipart"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/image/tiff"

	"phum-panya/internal/media"
)

// randomJPEG returns JPEG bytes for a w x h image filled with random noise,
// so lossy compression cannot shrink it much (used to force a large upload).
func randomJPEG(t *testing.T, w, h int) []byte {
	t.Helper()

	img := stdimage.NewNRGBA(stdimage.Rect(0, 0, w, h))
	if _, err := rand.Read(img.Pix); err != nil {
		t.Fatalf("rand.Read: %v", err)
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 100}); err != nil {
		t.Fatalf("jpeg.Encode: %v", err)
	}
	return buf.Bytes()
}

func decodedSize(t *testing.T, path string) (w, h int, format string) {
	t.Helper()

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("os.Open: %v", err)
	}
	defer f.Close()

	cfg, format, err := stdimage.DecodeConfig(f)
	if err != nil {
		t.Fatalf("stdimage.DecodeConfig: %v", err)
	}
	return cfg.Width, cfg.Height, format
}

func TestSaveReaderDownscalesPNGToJPEG(t *testing.T) {
	dir := t.TempDir()
	store := &media.Store{Dir: dir}

	src := stdimage.NewNRGBA(stdimage.Rect(0, 0, 3000, 2000))
	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, src); err != nil {
		t.Fatalf("encode PNG: %v", err)
	}

	relPath, err := store.SaveReader(&pngBuf)
	if err != nil {
		t.Fatalf("SaveReader: %v", err)
	}

	w, h, format := decodedSize(t, filepath.Join(dir, relPath))
	if format != "jpeg" {
		t.Errorf("format = %q, want jpeg", format)
	}
	if w > 1600 || h > 1600 {
		t.Errorf("size = %dx%d, want longest side <= 1600", w, h)
	}
}

func TestSaveReaderRejectsTIFF(t *testing.T) {
	dir := t.TempDir()
	store := &media.Store{Dir: dir}

	src := stdimage.NewNRGBA(stdimage.Rect(0, 0, 64, 64))
	var tiffBuf bytes.Buffer
	if err := tiff.Encode(&tiffBuf, src, nil); err != nil {
		t.Fatalf("tiff.Encode: %v", err)
	}

	if _, err := store.SaveReader(&tiffBuf); err == nil {
		t.Fatal("SaveReader accepted a TIFF, want rejection (NFR-IMG-1: JPEG/PNG/WebP only)")
	}
}

func TestSaveMultipartLargeJPEG(t *testing.T) {
	dir := t.TempDir()
	store := &media.Store{Dir: dir}

	jpegBytes := randomJPEG(t, 4000, 3000)
	if len(jpegBytes) <= 8<<20 {
		t.Fatalf("test fixture too small: %d bytes, want > 8MB", len(jpegBytes))
	}

	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, err := w.CreateFormFile("file", "big.jpg")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := part.Write(jpegBytes); err != nil {
		t.Fatalf("write part: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	reader := multipart.NewReader(&body, w.Boundary())
	form, err := reader.ReadForm(1 << 20)
	if err != nil {
		t.Fatalf("ReadForm: %v", err)
	}
	defer form.RemoveAll()

	fh := form.File["file"][0]

	relPath, err := store.SaveMultipart(fh)
	if err != nil {
		t.Fatalf("SaveMultipart: %v", err)
	}

	w2, h2, format := decodedSize(t, filepath.Join(dir, relPath))
	if format != "jpeg" {
		t.Errorf("format = %q, want jpeg", format)
	}
	if w2 > 1600 || h2 > 1600 {
		t.Errorf("size = %dx%d, want longest side <= 1600", w2, h2)
	}
}

func TestUsageBytes(t *testing.T) {
	dir := t.TempDir()
	store := &media.Store{Dir: dir}

	src := stdimage.NewNRGBA(stdimage.Rect(0, 0, 100, 100))
	if _, err := rand.Read(src.Pix); err != nil {
		t.Fatalf("rand.Read: %v", err)
	}
	var jpegBuf bytes.Buffer
	if err := jpeg.Encode(&jpegBuf, src, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatalf("jpeg.Encode: %v", err)
	}

	if _, err := store.SaveReader(&jpegBuf); err != nil {
		t.Fatalf("SaveReader: %v", err)
	}

	got, err := store.UsageBytes()
	if err != nil {
		t.Fatalf("UsageBytes: %v", err)
	}
	if got <= 0 {
		t.Errorf("UsageBytes = %d, want > 0", got)
	}

	missing := &media.Store{Dir: filepath.Join(dir, "does-not-exist")}
	got, err = missing.UsageBytes()
	if err != nil {
		t.Errorf("UsageBytes on missing dir: err = %v, want nil", err)
	}
	if got != 0 {
		t.Errorf("UsageBytes on missing dir = %d, want 0", got)
	}
}
