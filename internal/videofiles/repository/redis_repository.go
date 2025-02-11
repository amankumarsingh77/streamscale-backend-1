package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/amankumarsingh77/cloud-video-encoder/internal/models"
	"github.com/amankumarsingh77/cloud-video-encoder/internal/videofiles"
	"github.com/go-redis/redis/v8"
	"time"
)

type videoRedisRepo struct {
	redisClient *redis.Client
}

func NewVideoRedisRepo(redisClient *redis.Client) videofiles.RedisRepository {
	return &videoRedisRepo{
		redisClient: redisClient,
	}
}

func (v *videoRedisRepo) EnqueueJob(ctx context.Context, key string, videoJob *models.EncodeJob) error {
	return v.redisClient.LPush(ctx, key, videoJob).Err()
}

func (v *videoRedisRepo) PeekJob(ctx context.Context, key string) (*models.EncodeJob, error) {
	res, err := v.redisClient.LIndex(ctx, key, -1).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get job from queue: %w", err)
	}

	job := &models.EncodeJob{}
	if err = json.Unmarshal([]byte(res), job); err != nil {
		return nil, fmt.Errorf("error unmarshalling job: %w", err)
	}

	lockKey := "lock:" + job.JobID
	locked, err := v.redisClient.SetNX(ctx, lockKey, 1, 10*time.Minute).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to set lock: %w", err)
	}
	if !locked {
		return nil, fmt.Errorf("job %s is already being processed", job.JobID)
	}

	job.StartedAt = time.Now()
	job.Status = models.JobStatusProcessing
	updatedJobData, err := json.Marshal(job)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal updated job: %w", err)
	}

	// Update job data in Redis
	if err := v.redisClient.LSet(ctx, key, -1, string(updatedJobData)).Err(); err != nil {
		return nil, fmt.Errorf("failed to update job in queue: %w", err)
	}

	return job, nil
}

func (v *videoRedisRepo) DequeueJob(ctx context.Context, key string) (*models.EncodeJob, error) {
	res, err := v.redisClient.BLPop(ctx, 0*time.Second, key).Result()
	if err != nil {
		return nil, err
	}
	job := &models.EncodeJob{}
	if err = json.Unmarshal([]byte(res[1]), job); err != nil {
		return nil, fmt.Errorf("error unmarshalling job: %v", err)
	}
	job.StartedAt = time.Now()
	job.Status = models.JobStatusProcessing
	if err := v.UpdateStatus(ctx, job.JobID, "video:progress:", models.JobStatusProcessing); err != nil {
		return nil, fmt.Errorf("error updating job status: %v", err)
	}
	return job, nil
}

func (v *videoRedisRepo) UpdateProgress(ctx context.Context, jobID string, key string, progress float64) error {
	progressKey := key + jobID

	// Update progress in Redis hash
	err := v.redisClient.HSet(ctx, progressKey, "progress", progress).Err()
	if err != nil {
		return fmt.Errorf("failed to update progress: %w", err)
	}

	return nil
}

func (v *videoRedisRepo) UpdateStatus(ctx context.Context, jobID string, key string, status models.JobStatus) error {
	progressKey := key + jobID

	jobData, err := v.redisClient.HGet(ctx, progressKey, "job_data").Result()
	if err != nil {
		return fmt.Errorf("failed to get job data: %w", err)
	}

	var job models.EncodeJob
	if err := json.Unmarshal([]byte(jobData), &job); err != nil {
		return fmt.Errorf("failed to unmarshal job data: %w", err)
	}

	job.Status = status
	if status == models.JobStatusCompleted || status == models.JobStatusFailed {
		job.CompletedAt = time.Now()
	}

	updatedJobData, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("failed to marshal updated job: %w", err)
	}

	pipe := v.redisClient.Pipeline()
	pipe.HSet(ctx, progressKey, "status", status)
	pipe.HSet(ctx, progressKey, "job_data", string(updatedJobData))

	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to update status: %w", err)
	}

	return nil
}

func (v *videoRedisRepo) GetJobStatus(ctx context.Context, key string, jobID string) (models.JobStatus, error) {
	status, err := v.redisClient.HGet(ctx, key+jobID, "status").Result()
	if err != nil {
		return "", fmt.Errorf("failed to get job status: %w", err)
	}

	return models.JobStatus(status), nil
}
