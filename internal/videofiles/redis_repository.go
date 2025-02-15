package videofiles

import (
	"context"
	"github.com/amankumarsingh77/cloud-video-encoder/internal/models"
)

type RedisRepository interface {
	EnqueueJob(ctx context.Context, key string, videoJob *models.EncodeJob) error
	PeekJob(ctx context.Context, key string) (*models.EncodeJob, error)
	SubscribeToJobs(ctx context.Context, key string) (<-chan *models.EncodeJob, error)

	DequeueJob(ctx context.Context, key string) (*models.EncodeJob, error)
	GetJobStatus(ctx context.Context, key string, jobID string) (models.JobStatus, error)

	UpdateProgress(ctx context.Context, jobID string, key string, progress float64) error
	UpdateStatus(ctx context.Context, jobID string, key string, status models.JobStatus) error
}
