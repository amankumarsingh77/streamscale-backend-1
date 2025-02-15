package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"github.com/opencontainers/runtime-spec/specs-go"
	"log"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/containerd/cgroups"
	"github.com/go-redis/redis/v8"
)

// FFmpegTask represents a video processing task
type FFmpegTask struct {
	ID         string   `json:"id"`
	InputPath  string   `json:"input_path"`
	OutputPath string   `json:"output_path"`
	CPUPercent int      `json:"cpu_percent"` // CPU percentage limit (1-100)
	FFmpegArgs []string `json:"ffmpeg_args"`
}

// Worker represents a worker that can process tasks
type Worker struct {
	ID     string
	client *redis.Client
	quit   chan bool
	cgroup cgroups.Cgroup
}

// WorkerPool manages multiple workers
type WorkerPool struct {
	workers []*Worker
	client  *redis.Client
	wg      sync.WaitGroup
}

// NewWorkerPool creates a new worker pool with the specified number of workers
func NewWorkerPool(numWorkers int, redisAddr string) (*WorkerPool, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     "intimate-racer-53028.upstash.io:6379",
		Password: "Ac8kAAIjcDE2N2JmODcxY2U1MzI0MWU5OTA3MGY5YjM0Y2FjMjIxN3AxMA",
		DB:       0,
		TLSConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	})

	pool := &WorkerPool{
		workers: make([]*Worker, numWorkers),
		client:  client,
	}

	// Initialize workers
	for i := 0; i < numWorkers; i++ {
		// Use a static, relative path (do not include the mountpoint)
		cgroupPath := fmt.Sprintf("/ffmpeg-worker-%d", i)
		cg, err := createWorkerCgroup(cgroupPath)
		if err != nil {
			return nil, fmt.Errorf("failed to create cgroup: %v", err)
		}

		pool.workers[i] = &Worker{
			ID:     fmt.Sprintf("worker-%d", i),
			client: client,
			quit:   make(chan bool),
			cgroup: cg,
		}
	}

	return pool, nil
}

// createWorkerCgroup creates a new cgroup for CPU control.
// It selects the appropriate hierarchy based on the system's cgroup mode.
func createWorkerCgroup(path string) (cgroups.Cgroup, error) {
	shares := uint64(1024)
	// For containerd/cgroups v1, use cgroups.V1 as the hierarchy.
	control, err := cgroups.New(
		cgroups.V1,
		cgroups.StaticPath(path),
		&specs.LinuxResources{
			CPU: &specs.LinuxCPU{
				Shares: &shares,
			},
		},
	)
	if err != nil {
		return nil, err
	}
	return control, nil
}

// uint64Ptr returns a pointer to a uint64 value.
func uint64Ptr(val uint64) *uint64 {
	return &val
}

// Start begins the worker pool operations
func (wp *WorkerPool) Start(ctx context.Context) {
	for _, worker := range wp.workers {
		wp.wg.Add(1)
		go func(w *Worker) {
			defer wp.wg.Done()
			w.start(ctx)
		}(worker)
	}
}

// Stop gracefully shuts down the worker pool
func (wp *WorkerPool) Stop() {
	for _, worker := range wp.workers {
		worker.quit <- true
		if worker.cgroup != nil {
			worker.cgroup.Delete()
		}
	}
	wp.wg.Wait()
	wp.client.Close()
}

// start begins the worker's task processing loop
func (w *Worker) start(ctx context.Context) {
	for {
		select {
		case <-w.quit:
			return
		default:
			result, err := w.client.BLPop(ctx, 0, "ffmpeg_queue").Result()
			if err != nil {
				if err != redis.Nil {
					log.Printf("Worker %s: Error getting task: %v", w.ID, err)
				}
				continue
			}

			if len(result) > 1 {
				var task FFmpegTask
				if err := json.Unmarshal([]byte(result[1]), &task); err != nil {
					log.Printf("Worker %s: Error unmarshaling task: %v", w.ID, err)
					continue
				}
				w.processFFmpegTask(&task)
			}
		}
	}
}

// processFFmpegTask handles FFmpeg video processing
func (w *Worker) processFFmpegTask(task *FFmpegTask) {
	log.Printf("Worker %s: Processing video task: %s", w.ID, task.ID)

	// Prepare FFmpeg command
	args := append([]string{"-i", task.InputPath}, task.FFmpegArgs...)
	args = append(args, task.OutputPath)
	cmd := exec.Command("ffmpeg", args...)

	// Set up logging
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Start the FFmpeg process
	if err := cmd.Start(); err != nil {
		log.Printf("Worker %s: Failed to start FFmpeg: %v", w.ID, err)
		return
	}

	// Add the process to the worker's cgroup
	if err := w.cgroup.Add(cgroups.Process{Pid: cmd.Process.Pid}); err != nil {
		log.Printf("Worker %s: Failed to add process to cgroup: %v", w.ID, err)
	}

	// Update CPU limit based on task requirements
	if err := w.updateCPULimit(task.CPUPercent); err != nil {
		log.Printf("Worker %s: Failed to update CPU limit: %v", w.ID, err)
	}

	// Wait for the process to complete
	if err := cmd.Wait(); err != nil {
		log.Printf("Worker %s: FFmpeg process failed: %v", w.ID, err)
		return
	}

	log.Printf("Worker %s: Completed video task: %s", w.ID, task.ID)
}

// updateCPULimit updates the CPU limit for the worker's cgroup
func (w *Worker) updateCPULimit(cpuPercent int) error {
	// Convert percentage to cgroup cpu.shares value (2-262144)
	// 100% = 1024 shares
	shares := uint64(1024 * cpuPercent / 100)
	if shares < 2 {
		shares = 2
	}

	return w.cgroup.Update(&specs.LinuxResources{
		CPU: &specs.LinuxCPU{
			Shares: uint64Ptr(shares),
		},
	})
}

// submitFFmpegTask submits a task to the Redis queue.
func submitFFmpegTask(ctx context.Context, client *redis.Client, task *FFmpegTask) error {
	taskJSON, err := json.Marshal(task)
	if err != nil {
		return err
	}

	return client.RPush(ctx, "ffmpeg_queue", taskJSON).Err()
}

func main() {
	ctx := context.Background()

	// Create a worker pool with 3 workers
	pool, err := NewWorkerPool(3, "localhost:6379")
	if err != nil {
		log.Fatalf("Failed to create worker pool: %v", err)
	}

	// Start the worker pool
	pool.Start(ctx)

	// Example task submission
	task := &FFmpegTask{
		ID:         "task-1",
		InputPath:  "hackathon.mp4",
		OutputPath: "output.mp4",
		CPUPercent: 50,
		FFmpegArgs: []string{
			"-c:v", "libx264",
			"-preset", "medium",
			"-crf", "23",
		},
	}

	if err := submitFFmpegTask(ctx, pool.client, task); err != nil {
		log.Printf("Error submitting task: %v", err)
	}

	// Let tasks process for a while
	time.Sleep(30 * time.Second)

	// Gracefully shut down the worker pool
	pool.Stop()
}
