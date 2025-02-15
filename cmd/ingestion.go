package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"github.com/go-redis/redis/v8"
	"log"
	"time"
)

type EncodeJob struct {
	JobID                  string             `json:"job_id" redis:"job_id"`
	UserID                 string             `json:"user_id" redis:"user_id"`
	VideoID                string             `json:"video_id" redis:"video_id"`
	InputS3Key             string             `json:"input_s3_key" redis:"input_s3_key"`
	InputBucket            string             `json:"input_bucket" redis:"input_bucket"`
	Progress               float64            `json:"progress" redis:"progress"`
	OutputS3Key            string             `json:"output_s3_key" redis:"output_s3_key"`
	OutputBucket           string             `json:"output_bucket" redis:"output_bucket"`
	Qualities              []InputQualityInfo `json:"qualities" redis:"qualities"`
	OutputFormats          []PlaybackFormat   `json:"output_formats" redis:"output_formats"`
	EnablePerTitleEncoding bool               `json:"enable_per_title_encoding" redis:"enable_per_title_encoding"`
	Status                 JobStatus          `json:"status" redis:"status"`
	StartedAt              time.Time          `json:"started_at" redis:"started_at"`
	CompletedAt            time.Time          `json:"completed_at" redis:"completed_at"`
}

type InputQualityInfo struct {
	// Define fields as per your requirements
}

type PlaybackFormat struct {
	// Define fields as per your requirements
}

type JobStatus string

const (
	JobStatusQueued     JobStatus = "video_jobs"
	JobStatusProcessing JobStatus = "in_progress"
	JobStatusCompleted  JobStatus = "completed"
	JobStatusFailed     JobStatus = "failed"
)

func main() {
	rdb := redis.NewClient(&redis.Options{
		Addr:     "intimate-racer-53028.upstash.io:6379",
		Password: "Ac8kAAIjcDE2N2JmODcxY2U1MzI0MWU5OTA3MGY5YjM0Y2FjMjIxN3AxMA",
		DB:       0,
		TLSConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	})

	job := EncodeJob{
		JobID:                  "12345",
		UserID:                 "user_001",
		VideoID:                "video_001",
		InputS3Key:             "BitBloom.mp4",
		InputBucket:            "input-bucket",
		Progress:               0.0,
		OutputS3Key:            "temp/output/video_001_encoded.mp4",
		OutputBucket:           "output-bucket",
		Qualities:              []InputQualityInfo{},
		OutputFormats:          []PlaybackFormat{},
		EnablePerTitleEncoding: false,
		Status:                 JobStatusQueued,
		StartedAt:              time.Now(),
		CompletedAt:            time.Time{},
	}

	ctx := context.Background()

	jobJSON, err := json.Marshal(job)
	if err != nil {
		log.Fatalf("Error marshalling job: %v", err)
	}

	channel := "video_jobs"
	err = rdb.Publish(ctx, channel, jobJSON).Err()
	if err != nil {
		log.Fatalf("Error publishing job to Redis: %v", err)
	}

	fmt.Printf("Job published successfully to Redis channel: %s\n", channel)
}
