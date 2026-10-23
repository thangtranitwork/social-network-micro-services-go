package imaging

import (
	"context"
	"fmt"
	"io"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type MinIOReader struct {
	Client *minio.Client
	Bucket string
}

func NewMinIOReader(endpoint, accessKey, secretKey, bucket string, useSSL bool) (*MinIOReader, error) {
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to init minio client: %w", err)
	}

	return &MinIOReader{
		Client: client,
		Bucket: bucket,
	}, nil
}

// FetchImage downloads image file bytes from MinIO with a 10MB safety limit.
func (m *MinIOReader) FetchImage(ctx context.Context, fileID string) ([]byte, error) {
	if m.Client == nil {
		return nil, fmt.Errorf("minio client is nil")
	}
	if fileID == "" {
		return nil, fmt.Errorf("empty fileID")
	}

	obj, err := m.Client.GetObject(ctx, m.Bucket, fileID, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get minio object %s: %w", fileID, err)
	}
	defer obj.Close()

	// Limit to max 10MB to prevent memory exhaustion
	limitReader := io.LimitReader(obj, 10*1024*1024)
	data, err := io.ReadAll(limitReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read minio object bytes %s: %w", fileID, err)
	}

	if len(data) == 0 {
		return nil, fmt.Errorf("empty image bytes for file %s", fileID)
	}

	return data, nil
}
