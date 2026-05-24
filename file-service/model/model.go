package model

import "time"

type File struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	ContentType string    `json:"contentType"`
	UploaderID  string    `json:"uploaderId"`
	CreatedAt   time.Time `json:"createdAt"`
}

type FileResponse struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	ContentType string `json:"contentType"`
	URL         string `json:"url"` // Presigned URL or public URL
}
