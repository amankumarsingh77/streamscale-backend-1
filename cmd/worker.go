package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/amankumarsingh77/cloud-video-encoder/internal/config"
	"github.com/amankumarsingh77/cloud-video-encoder/internal/models"
	"github.com/amankumarsingh77/cloud-video-encoder/internal/videofiles"
	"github.com/go-redis/redis/v8"
	"log"
	"sync"
	"time"
)

type Worker struct {
	cfg       *config.Config
	redis     *redis.Client
	s3Client  *videofiles.AWSRepository
	processor *processor.Encoder
	wg        *sync.WaitGroup
}

func NewWorker(cfg *config.Config, redis *redis.Client, s3Client *videofiles.AWSRepository, processor *processor.Encoder, wg *sync.WaitGroup) *Worker {
	return &Worker{
		cfg:       cfg,
		redis:     redis,
		s3Client:  s3Client,
		processor: processor,
		wg:        wg,
	}
}

func (w *Worker) Start(ctx context.Context) error {
	for i := 0; i < w.cfg.Worker.WorkerCount; i++ {
		w.wg.Add(1)
		go w.processJobs(ctx)
	}
	w.wg.Wait()
	return nil
}

func (w *Worker) processJobs(ctx context.Context) {
	defer w.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		default:
			result, err := w.redis.BLPop(ctx, 0, w.cfg.Redis.JobQueueKey).Result()
			if err != nil {
				log.Printf("Error polling Redis: %v", err)
				continue
			}

			if len(result) != 2 {
				continue
			}

			var job models.EncodeJob
			if err := json.Unmarshal([]byte(result[1]), &job); err != nil {
				log.Printf("Error unmarshaling job: %v", err)
				continue
			}

			if err := w.processJob(ctx, &job); err != nil {
				log.Printf("Error processing job %s: %v", job.JobID, err)
				job.Status = "failed"
			}

			jobBytes, _ := json.Marshal(job)
			w.redis.Set(ctx, fmt.Sprintf("job:%s", job.JobID), jobBytes, 24*time.Hour)
		}
	}
}
