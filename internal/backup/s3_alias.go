package backup

import "github.com/eyupio/reet.sh/internal/objectstore"

type s3Client = objectstore.Client

func newS3Client(c S3) (*s3Client, error) {
	return objectstore.New(objectstore.Config{
		Endpoint: c.Endpoint, Bucket: c.Bucket, Region: c.Region,
		AccessKey: c.AccessKey, SecretKey: c.SecretKey, PathStyle: c.PathStyle,
	})
}
