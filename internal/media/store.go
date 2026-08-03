// Package media stores uploaded images as downscaled, EXIF-stripped JPEGs
// addressed by content hash.
package media

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"mime/multipart"
	"os"
	"path/filepath"

	"github.com/disintegration/imaging"
	_ "golang.org/x/image/webp" // register WebP decoder
)

// maxDimension is the longest allowed side, in pixels, of a stored image.
const maxDimension = 1600

// jpegQuality is the quality used when re-encoding stored images.
const jpegQuality = 80

// Store saves images under Dir.
type Store struct {
	Dir string
}

// SaveReader decodes an image from r (JPEG, PNG, or WebP), downscales its
// longest side to maxDimension, and re-encodes it as JPEG. Re-encoding
// drops EXIF metadata, including GPS. It stores the result under Dir at a
// path derived from the SHA-256 of the encoded bytes and returns that path
// relative to Dir.
func (s *Store) SaveReader(r io.Reader) (string, error) {
	img, err := imaging.Decode(r)
	if err != nil {
		return "", fmt.Errorf("media: decode image: %w", err)
	}

	resized := imaging.Fit(img, maxDimension, maxDimension, imaging.Lanczos)

	var buf bytes.Buffer
	if err := imaging.Encode(&buf, resized, imaging.JPEG, imaging.JPEGQuality(jpegQuality)); err != nil {
		return "", fmt.Errorf("media: encode image: %w", err)
	}

	sum := sha256.Sum256(buf.Bytes())
	hexSum := hex.EncodeToString(sum[:])
	relPath := filepath.Join(hexSum[:2], hexSum+".jpg")

	fullPath := filepath.Join(s.Dir, relPath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return "", fmt.Errorf("media: create dir: %w", err)
	}
	if err := os.WriteFile(fullPath, buf.Bytes(), 0o644); err != nil {
		return "", fmt.Errorf("media: write file: %w", err)
	}

	return relPath, nil
}

// SaveMultipart opens the uploaded file in fh and stores it via SaveReader.
// Gin streams large multipart parts to a temp file rather than buffering
// them in memory, so this does not load the whole upload into memory.
// There is no size cap.
func (s *Store) SaveMultipart(fh *multipart.FileHeader) (string, error) {
	f, err := fh.Open()
	if err != nil {
		return "", fmt.Errorf("media: open upload: %w", err)
	}
	defer f.Close()

	return s.SaveReader(f)
}

// UsageBytes returns the total size, in bytes, of all regular files under
// Dir. It returns 0, nil if Dir does not exist.
func (s *Store) UsageBytes() (int64, error) {
	var total int64
	err := filepath.WalkDir(s.Dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type().IsRegular() {
			info, err := d.Info()
			if err != nil {
				return err
			}
			total += info.Size()
		}
		return nil
	})
	if os.IsNotExist(err) {
		return 0, nil
	}
	return total, err
}
