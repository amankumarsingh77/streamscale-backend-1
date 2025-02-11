package main

import (
	"context"
	"github.com/amankumarsingh77/cloud-video-encoder/internal/config"
	"github.com/amankumarsingh77/cloud-video-encoder/internal/videofiles"
	"github.com/amankumarsingh77/cloud-video-encoder/internal/videofiles/repository"
	"github.com/amankumarsingh77/cloud-video-encoder/internal/worker"
	"github.com/amankumarsingh77/cloud-video-encoder/pkg/db/aws"
	"github.com/amankumarsingh77/cloud-video-encoder/pkg/db/postgres"
	clientRedis "github.com/amankumarsingh77/cloud-video-encoder/pkg/db/redis"
	"github.com/amankumarsingh77/cloud-video-encoder/pkg/logger"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const (
	redisAddr     = "localhost:6379"
	queueName     = "video_jobs"
	maxCPUUsage   = 80.0
	checkInterval = 10 * time.Second
)

func main() {
	configFile := "config.yml"
	cfgFile, err := config.LoadConfig(configFile)

	if err != nil {
		log.Fatalf("loadConfig: %v", err)
	}
	cfg, err := config.ParseConfig(cfgFile)
	if err != nil {
		log.Fatalf("parseConfig: %v", err)
	}
	appLogger := logger.NewApiLogger(cfg)
	appLogger.InitLogger()
	appLogger.Infof("AppVersion: %s, LogLevel: %s, Mode: %s", cfg.Server.AppVersion, cfg.Logger.Level, cfg.Server.Mode)
	psqlDB, err := postgres.NewPsqlDB(cfg)
	if err != nil {
		appLogger.Infof("could not connect to db: %s", err)
	}
	appLogger.Infof("db connected, status: %#v", psqlDB.Stats())
	defer psqlDB.Close()

	redisClient, err := clientRedis.NewRedisClient(cfg)
	if err != nil {
		appLogger.Infof("could not connect to redis: %s", err)
	}
	appLogger.Infof("redis connected")

	awsClient, presignClient, err := aws.NewAWSClient(cfg.S3.Endpoint, cfg.S3.Region, cfg.S3.AccessKey, cfg.S3.SecretKey)
	if err != nil {
		appLogger.Infof("could not connect to s3: %s", err)
		return
	}

	awsRepo := repository.NewAwsRepository(awsClient, presignClient)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	videoRedisClient := repository.NewVideoRedisRepo(redisClient)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Shutting down...")
		cancel()
	}()

	for {
		select {
		case <-ctx.Done():
			return
		default:
			if shouldProcess, usage := checkCPU(); shouldProcess {
				processJob(ctx, videoRedisClient, cfg.S3.InputBucket, awsRepo)
			} else {
				log.Printf("CPU usage %.2f%% too high, waiting...", usage)
				time.Sleep(checkInterval)
			}
		}
	}
}

func checkCPU() (bool, float64) {
	usage, err := worker.GetCPUUsage()
	if err != nil {
		log.Printf("CPU check error: %v", err)
		return false, 0
	}
	return usage <= maxCPUUsage, usage
}

func processJob(ctx context.Context, client videofiles.RedisRepository, bucket string, awsRepo videofiles.AWSRepository) {
	job, err := client.PeekJob(ctx, "video_jobs")
	if err != nil {
		log.Printf("Failed to fetch job: %v", err)
		return
	}

	//tempDir, err := os.MkdirTemp("", "video_job_")
	//if err != nil {
	//	log.Printf("Failed to create temp dir: %v", err)
	//	return
	//}

	log.Printf("Processing job %s", job.VideoID)
	if err := worker.Process(ctx, job.InputS3Key, job.OutputS3Key, "temp", bucket, awsRepo); err != nil {
		log.Printf("Job %s failed: %v", job.VideoID, err)
	} else {
		log.Printf("Job %s completed successfully", job.VideoID)
	}
}
