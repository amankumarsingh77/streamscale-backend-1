package worker

import (
	"context"
	"fmt"
	"github.com/amankumarsingh77/cloud-video-encoder/internal/videofiles"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"
)

const (
	VideoJobsQueueKey = "video_jobs"
)

func StartJob(ctx context.Context, inputBucket, outputBucket string, videRedisClient videofiles.RedisRepository, awsRepo videofiles.AWSRepository) error {
	job, err := videRedisClient.PeekJob(ctx, VideoJobsQueueKey)
	if err != nil {
		return err
	}
	defer func() {
		log.Println("Cleaning temporary files...")
		os.RemoveAll(TempDir)
	}()
	//Download the video
	localVideoPath, err := downloadVideo(ctx, awsRepo, inputBucket, job.InputS3Key)
	if err != nil {
		return err
	}

	//Process the video
	if err := ProcessVideo(ctx, localVideoPath, "output"); err != nil {
		return err
	}
	return nil

}

func downloadVideo(ctx context.Context, awsRepo videofiles.AWSRepository, bucket, key string) (string, error) {
	err := os.MkdirAll(TempDir, os.ModePerm)
	if err != nil {
		return "", fmt.Errorf("failed to create temp directory: %w", err)
	}
	localPath := filepath.Join(TempDir, filepath.Base(key))
	videoFile, err := awsRepo.GetObject(ctx, bucket, key)
	if err != nil {
		return "", fmt.Errorf("failed to get object from S3: %w", err)
	}
	defer videoFile.Body.Close()
	outFile, err := os.Create(localPath)
	if err != nil {
		return "", fmt.Errorf("failed to create local video file: %w", err)
	}
	defer outFile.Close()
	_, err = io.Copy(outFile, videoFile.Body)
	if err != nil {
		return "", fmt.Errorf("failed to write video file: %w", err)
	}
	return localPath, nil
}

func ProcessVideo(ctx context.Context, inputPath, outputPath string) error {
	defer func() {
		log.Println("Cleaning temporary files...")
		os.RemoveAll(TempDir)
	}()
	videoInfo, err := GetVideoInfo(inputPath)
	if err != nil {
		return fmt.Errorf("failed to get video info: %w", err)
	}
	segments, err := SplitVideo(inputPath, TempDir, videoInfo)
	if err != nil {
		return fmt.Errorf("splitting failed: %w", err)
	}
	spatial, temporal, err := AnalyzeComplexity(segments[0])
	if err != nil {
		return fmt.Errorf("complexity analysis failed: %w", err)
	}
	bitrate := ComputeBitrate(videoInfo, spatial, temporal)
	encodedSegments, err := ParallelEncodeSegments(segments, bitrate)
	if err != nil {
		return fmt.Errorf("encoding failed: %w", err)
	}
	if err := StitchSegments(encodedSegments, outputPath); err != nil {
		return fmt.Errorf("stitching failed: %w", err)
	}
	// dir := os.TempDir()
	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current working directory: %w", err)
	}
	finalOutput := filepath.Join(dir, "finalOutput")
	if err := PackageVideo(inputPath, finalOutput); err != nil {
		return fmt.Errorf("packaging failed: %w", err)
	}
	return nil
}
func Process(ctx context.Context, inputPath, outputPath, tempDir string, bucket string, awsRepo videofiles.AWSRepository) error {
	start := time.Now()
	//defer func() {
	//	log.Println("Cleaning temporary files...")
	//	os.RemoveAll(tempDir)
	//}()

	//localVideoPath := filepath.Join(tempDir, filepath.Base(inputPath))
	localVideoPath := filepath.Join(tempDir, "big_buck_bunny_1080p_h264.mov")
	//log.Println(awsRepo.ListObjects(ctx, bucket))
	//videoFile, err := awsRepo.GetObject(ctx, bucket, inputPath)
	//if err != nil {
	//	return fmt.Errorf("failed to get object from S3: %w", err)
	//}
	//defer videoFile.Body.Close()
	//
	//outFile, err := os.Create(localVideoPath)
	//if err != nil {
	//	return fmt.Errorf("failed to create local video file: %w", err)
	//}
	//defer outFile.Close()
	//
	//if _, err := io.Copy(outFile, videoFile.Body); err != nil {
	//	return fmt.Errorf("failed to write video file: %w", err)
	//}
	//
	//log.Println("Video downloaded successfully:", localVideoPath)

	videoInfo, err := GetVideoInfo(localVideoPath)
	if err != nil {
		return fmt.Errorf("failed to get video info: %w", err)
	}

	segments, err := SplitVideo(localVideoPath, tempDir, videoInfo)
	if err != nil {
		return fmt.Errorf("splitting failed: %w", err)
	}

	spatial, temporal, err := AnalyzeComplexity(segments[0])
	if err != nil {
		return fmt.Errorf("complexity analysis failed: %w", err)
	}

	bitrate := ComputeBitrate(videoInfo, spatial, temporal)
	encodedSegments, err := ParallelEncodeSegments(segments, bitrate)
	if err != nil {
		return fmt.Errorf("encoding failed: %w", err)
	}

	if err := StitchSegments(encodedSegments, outputPath); err != nil {
		return fmt.Errorf("stitching failed: %w", err)
	}

	log.Printf("Job completed in %s", time.Since(start))
	return nil
}
