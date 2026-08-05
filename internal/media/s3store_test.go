package media

import (
	"bytes"
	"context"
	"errors"
	stdimage "image"
	"image/png"
	"io"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// tinyPNG returns PNG-encoded bytes for a 2x2 image, a minimal valid,
// decodable fixture. This package uses the unexported s3API seam, so it
// cannot import the media_test package's tinyPNG helper.
func tinyPNG(t *testing.T) []byte {
	t.Helper()

	img := stdimage.NewRGBA(stdimage.Rect(0, 0, 2, 2))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png.Encode: %v", err)
	}
	return buf.Bytes()
}

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

func TestS3StoreIntegration(t *testing.T) {
	endpoint := os.Getenv("APP_S3_ENDPOINT")
	if endpoint == "" {
		t.Skip("set APP_S3_ENDPOINT to run the real-Garage integration test")
	}

	s, err := NewS3Store(context.Background(), endpoint,
		os.Getenv("APP_S3_REGION"), os.Getenv("APP_S3_ACCESS_KEY"),
		os.Getenv("APP_S3_SECRET_KEY"), os.Getenv("APP_S3_BUCKET"), true)
	if err != nil {
		t.Fatalf("NewS3Store: %v", err)
	}

	key, err := s.SaveReader(bytes.NewReader(tinyPNG(t)))
	if err != nil {
		t.Fatalf("SaveReader: %v", err)
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
