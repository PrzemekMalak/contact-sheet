// Package store uploads, deletes and copies original image files in S3,
// with a local no-op fallback when no bucket is configured.
package store

import (
	"context"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"strings"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/PrzemekMalak/contact-sheet/tool/internal/config"
)

// Store manages original image files by filename.
type Store interface {
	Upload(filename, localPath string) (url string, err error)
	Delete(filename string) error
	// DeleteKey deletes the object with this exact key (originals that older
	// versions uploaded into album subfolders, e.g. originals/daily/x.jpg).
	DeleteKey(key string) error
	Copy(srcFilename, dstFilename string) error
	Key(filename string) string
}

// New returns an S3-backed store when cfg.Bucket is set, else a local no-op store.
func New(cfg *config.Config) (Store, error) {
	if cfg.Bucket == "" {
		return &localStore{cfg: cfg}, nil
	}
	opts := []func(*awsconfig.LoadOptions) error{awsconfig.WithRegion(cfg.Region)}
	if cfg.Profile != "" {
		opts = append(opts, awsconfig.WithSharedConfigProfile(cfg.Profile))
	}
	ac, err := awsconfig.LoadDefaultConfig(context.Background(), opts...)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}
	return newS3(cfg, s3.NewFromConfig(ac)), nil
}

func key(cfg *config.Config, filename string) string {
	return cfg.OriginalsPrefix + "/" + filename
}

type localStore struct{ cfg *config.Config }

func (l *localStore) Upload(_, localPath string) (string, error) { return localPath, nil }
func (l *localStore) Delete(string) error                        { return nil }
func (l *localStore) DeleteKey(string) error                     { return nil }
func (l *localStore) Copy(_, _ string) error                     { return nil }
func (l *localStore) Key(filename string) string                 { return key(l.cfg, filename) }

// OriginalCacheControl is sent with every original. CloudFront and browsers
// keep a copy for a week. Not longer: deleting a photo and adding another file
// with the same name replaces the original under the same key.
const OriginalCacheControl = "public, max-age=604800"

// contentTypeFor guesses the MIME type from the file name. Without one, S3
// stores application/octet-stream and browsers download the original
// instead of showing it.
func contentTypeFor(filename string) string {
	if t := mime.TypeByExtension(strings.ToLower(filepath.Ext(filename))); t != "" {
		if i := strings.IndexByte(t, ';'); i >= 0 {
			t = strings.TrimSpace(t[:i])
		}
		return t
	}
	return "application/octet-stream"
}

// s3API is the subset of *s3.Client used here; tests fake it.
type s3API interface {
	PutObject(ctx context.Context, in *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	DeleteObject(ctx context.Context, in *s3.DeleteObjectInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectOutput, error)
	CopyObject(ctx context.Context, in *s3.CopyObjectInput, optFns ...func(*s3.Options)) (*s3.CopyObjectOutput, error)
}

type s3Store struct {
	cfg *config.Config
	api s3API
}

func newS3(cfg *config.Config, api s3API) *s3Store { return &s3Store{cfg: cfg, api: api} }

func (s *s3Store) Key(filename string) string { return key(s.cfg, filename) }

func (s *s3Store) Upload(filename, localPath string) (string, error) {
	f, err := os.Open(localPath)
	if err != nil {
		return "", err
	}
	defer f.Close()
	k := s.Key(filename)
	bucket := s.cfg.Bucket
	contentType, cacheControl := contentTypeFor(filename), OriginalCacheControl
	if _, err := s.api.PutObject(context.Background(), &s3.PutObjectInput{
		Bucket:       &bucket,
		Key:          &k,
		Body:         f,
		ContentType:  &contentType,
		CacheControl: &cacheControl,
	}); err != nil {
		return "", fmt.Errorf("put %s: %w", k, err)
	}
	return s.cfg.CDNBase + "/" + k, nil
}

func (s *s3Store) Delete(filename string) error { return s.DeleteKey(s.Key(filename)) }

func (s *s3Store) DeleteKey(k string) error {
	bucket := s.cfg.Bucket
	if _, err := s.api.DeleteObject(context.Background(), &s3.DeleteObjectInput{Bucket: &bucket, Key: &k}); err != nil {
		return fmt.Errorf("delete %s: %w", k, err)
	}
	return nil
}

func (s *s3Store) Copy(srcFilename, dstFilename string) error {
	bucket := s.cfg.Bucket
	dst := s.Key(dstFilename)
	src := bucket + "/" + s.Key(srcFilename)
	if _, err := s.api.CopyObject(context.Background(), &s3.CopyObjectInput{
		Bucket:     &bucket,
		Key:        &dst,
		CopySource: &src,
	}); err != nil {
		return fmt.Errorf("copy %s -> %s: %w", src, dst, err)
	}
	return nil
}
