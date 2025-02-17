package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/amankumarsingh77/cloud-video-encoder/internal/models"
	"github.com/amankumarsingh77/cloud-video-encoder/internal/videofiles"
	"github.com/go-redis/redis/v8"
	"log"
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
	// First, check if there are any jobs in the queue
	length, err := v.redisClient.LLen(ctx, key).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get queue length: %w", err)
	}

	if length == 0 {
		return nil, nil // No jobs available
	}

	// Get all jobs to find an unlocked one
	jobs, err := v.redisClient.LRange(ctx, key, 0, length-1).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get jobs from queue: %w", err)
	}

	for idx, jobData := range jobs {
		job := &models.EncodeJob{}
		if err = json.Unmarshal([]byte(jobData), job); err != nil {
			log.Printf("Error unmarshalling job at index %d: %v", idx, err)
			continue
		}
		if job.Status == models.JobStatusProcessing {
			continue
		}

		lockKey := "lock:" + job.JobID
		locked, err := v.redisClient.SetNX(ctx, lockKey, 1, 10*time.Minute).Result()
		if err != nil {
			log.Printf("Error setting lock for job %s: %v", job.JobID, err)
			continue
		}

		if !locked {
			continue
		}

		job.StartedAt = time.Now()
		job.Status = models.JobStatusProcessing
		updatedJobData, err := json.Marshal(job)
		if err != nil {
			// Release the lock if we fail to marshal
			v.redisClient.Del(ctx, lockKey)
			return nil, fmt.Errorf("failed to marshal updated job: %w", err)
		}

		// Update job data in Redis
		err = v.redisClient.LSet(ctx, key, int64(idx), string(updatedJobData)).Err()
		if err != nil {
			// Release the lock if we fail to update
			v.redisClient.Del(ctx, lockKey)
			return nil, fmt.Errorf("failed to update job in queue: %w", err)
		}

		log.Printf("Successfully locked and updated job %s at index %d", job.JobID, idx)
		return job, nil
	}

	// No available jobs found
	return nil, nil
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

func (v *videoRedisRepo) GetRedisClient() *redis.Client {
	return v.redisClient
}

func (v *videoRedisRepo) SubscribeToJobs(ctx context.Context, key string) *redis.PubSub {
	return v.redisClient.Subscribe(ctx, key)
}
