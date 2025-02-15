package main

import (
	"context"
	"github.com/amankumarsingh77/cloud-video-encoder/internal/config"
	"github.com/amankumarsingh77/cloud-video-encoder/internal/videofiles/repository"
	"github.com/amankumarsingh77/cloud-video-encoder/internal/worker"
	"github.com/amankumarsingh77/cloud-video-encoder/pkg/db/aws"
	"github.com/amankumarsingh77/cloud-video-encoder/pkg/db/postgres"
	clientRedis "github.com/amankumarsingh77/cloud-video-encoder/pkg/db/redis"
	"github.com/amankumarsingh77/cloud-video-encoder/pkg/logger"
	"github.com/amankumarsingh77/cloud-video-encoder/pkg/utils"
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
	// Load configuration
	configFile := "config.yml"
	cfgFile, err := config.LoadConfig(configFile)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	cfg, err := config.ParseConfig(cfgFile)
	if err != nil {
		log.Fatalf("Failed to parse config: %v", err)
	}

	// Initialize logger
	appLogger := logger.NewApiLogger(cfg)
	appLogger.InitLogger()
	appLogger.Infof("Starting worker service - Version: %s, LogLevel: %s, Mode: %s",
		cfg.Server.AppVersion, cfg.Logger.Level, cfg.Server.Mode)

	// Initialize PostgreSQL
	psqlDB, err := postgres.NewPsqlDB(cfg)
	if err != nil {
		appLogger.Fatalf("PostgreSQL init error: %s", err)
	}
	defer psqlDB.Close()
	appLogger.Info("PostgreSQL connected successfully")

	// Initialize Redis
	redisClient, err := clientRedis.NewRedisClient(cfg)
	if err != nil {
		appLogger.Fatalf("Redis init error: %s", err)
	}
	appLogger.Info("Redis connected successfully")

	// Initialize AWS clients
	awsClient, presignClient, err := aws.NewAWSClient(
		cfg.S3.Endpoint,
		cfg.S3.Region,
		cfg.S3.AccessKey,
		cfg.S3.SecretKey,
	)
	if err != nil {
		appLogger.Fatalf("AWS init error: %s", err)
	}
	appLogger.Info("AWS client initialized successfully")

	// Initialize repositories
	awsRepo := repository.NewAwsRepository(awsClient, presignClient)
	redisRepo := repository.NewVideoRedisRepo(redisClient)

	// Create context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize and start worker pool
	videoWorker := worker.NewWorker(cfg, appLogger, redisRepo, awsRepo)
	if err := videoWorker.Start(ctx); err != nil {
		appLogger.Fatalf("Failed to start worker: %s", err)
	}

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start health check routine
	//go runHealthCheck(ctx, appLogger, cfg)

	sig := <-sigChan
	appLogger.Infof("Received shutdown signal: %v", sig)

	// Initialize graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	// Stop the worker
	videoWorker.Stop()

	// Wait for shutdown context
	<-shutdownCtx.Done()
	appLogger.Info("Worker service stopped successfully")
}

func runHealthCheck(ctx context.Context, logger logger.Logger, cfg *config.Config) {
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, cpuUsage := utils.CheckCPUUsage(cfg.Worker.MaxCPUUsage)

			if cpuUsage > maxCPUUsage {
				logger.Warnf("High CPU usage detected: %.2f%%", cpuUsage)
			}

			// Add additional health checks here (Redis, Postgres, etc.)
			logger.Infof("Health check - CPU Usage: %.2f%%", cpuUsage)
		}
	}
}
