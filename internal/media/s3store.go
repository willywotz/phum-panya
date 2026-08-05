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

var _ Store = (*S3Store)(nil)

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
