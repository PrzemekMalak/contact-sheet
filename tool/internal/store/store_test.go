package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/PrzemekMalak/contact-sheet/tool/internal/config"
)

func testCfg() *config.Config {
	return &config.Config{
		Bucket:          "my-bucket",
		OriginalsPrefix: "originals",
		CDNBase:         "https://cdn.example.com",
		Region:          "us-east-1",
	}
}

type fakeS3 struct {
	put *s3.PutObjectInput
	del *s3.DeleteObjectInput
	cp  *s3.CopyObjectInput
}

func (f *fakeS3) PutObject(_ context.Context, in *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	f.put = in
	return &s3.PutObjectOutput{}, nil
}
func (f *fakeS3) DeleteObject(_ context.Context, in *s3.DeleteObjectInput, _ ...func(*s3.Options)) (*s3.DeleteObjectOutput, error) {
	f.del = in
	return &s3.DeleteObjectOutput{}, nil
}
func (f *fakeS3) CopyObject(_ context.Context, in *s3.CopyObjectInput, _ ...func(*s3.Options)) (*s3.CopyObjectOutput, error) {
	f.cp = in
	return &s3.CopyObjectOutput{}, nil
}

func TestLocalStoreUploadReturnsLocalPath(t *testing.T) {
	cfg := testCfg()
	cfg.Bucket = ""
	s, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.Upload("a.jpg", "assets/gallery/a-preview.webp")
	if err != nil || got != "assets/gallery/a-preview.webp" {
		t.Fatalf("got %q, %v", got, err)
	}
	if err := s.Delete("a.jpg"); err != nil {
		t.Fatal(err)
	}
	if err := s.Copy("a.jpg", "b.jpg"); err != nil {
		t.Fatal(err)
	}
}

func TestKeyFormat(t *testing.T) {
	cfg := testCfg()
	cfg.Bucket = ""
	s, _ := New(cfg)
	if got := s.Key("a.jpg"); got != "originals/a.jpg" {
		t.Fatalf("got %q", got)
	}
}

func TestS3UploadCallsPutObject(t *testing.T) {
	f := &fakeS3{}
	s := newS3(testCfg(), f)
	p := filepath.Join(t.TempDir(), "a.jpg")
	if err := os.WriteFile(p, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	url, err := s.Upload("a.jpg", p)
	if err != nil {
		t.Fatal(err)
	}
	if *f.put.Bucket != "my-bucket" || *f.put.Key != "originals/a.jpg" {
		t.Fatalf("bucket/key = %q/%q", *f.put.Bucket, *f.put.Key)
	}
	if url != "https://cdn.example.com/originals/a.jpg" {
		t.Fatalf("url = %q", url)
	}
}

func TestS3DeleteCallsDeleteObject(t *testing.T) {
	f := &fakeS3{}
	s := newS3(testCfg(), f)
	if err := s.Delete("a.jpg"); err != nil {
		t.Fatal(err)
	}
	if *f.del.Bucket != "my-bucket" || *f.del.Key != "originals/a.jpg" {
		t.Fatalf("bucket/key = %q/%q", *f.del.Bucket, *f.del.Key)
	}
}

func TestS3CopyCallsCopyObject(t *testing.T) {
	f := &fakeS3{}
	s := newS3(testCfg(), f)
	if err := s.Copy("a.jpg", "b.jpg"); err != nil {
		t.Fatal(err)
	}
	if *f.cp.Bucket != "my-bucket" || *f.cp.Key != "originals/b.jpg" || *f.cp.CopySource != "my-bucket/originals/a.jpg" {
		t.Fatalf("got %q %q %q", *f.cp.Bucket, *f.cp.Key, *f.cp.CopySource)
	}
}

// Without a Content-Type, S3 stores application/octet-stream and browsers
// download the original instead of showing it.
func TestS3UploadSetsContentTypeAndCacheControl(t *testing.T) {
	cases := map[string]string{"a.jpg": "image/jpeg", "b.jpeg": "image/jpeg", "c.png": "image/png", "d.unknownext": "application/octet-stream"}
	for name, want := range cases {
		f := &fakeS3{}
		s := newS3(testCfg(), f)
		p := filepath.Join(t.TempDir(), name)
		if err := os.WriteFile(p, []byte("data"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Upload(name, p); err != nil {
			t.Fatal(err)
		}
		if f.put.ContentType == nil || *f.put.ContentType != want {
			t.Errorf("%s: Content-Type %v, want %q", name, f.put.ContentType, want)
		}
		if f.put.CacheControl == nil || *f.put.CacheControl != OriginalCacheControl {
			t.Errorf("%s: Cache-Control %v, want %q", name, f.put.CacheControl, OriginalCacheControl)
		}
	}
}
