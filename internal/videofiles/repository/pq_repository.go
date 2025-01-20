package repository

import (
	"context"
	"fmt"
	"github.com/amankumarsingh77/cloud-video-encoder/internal/models"
	"github.com/amankumarsingh77/cloud-video-encoder/internal/videofiles"
	"github.com/amankumarsingh77/cloud-video-encoder/pkg/utils"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type videoRepo struct {
	db *sqlx.DB
}

func NewVideoRepo(db *sqlx.DB) videofiles.Repository {
	return &videoRepo{
		db: db,
	}
}

func (v *videoRepo) CreateVideo(ctx context.Context, videoFile *models.VideoFile) (*models.VideoFile, error) {
	video := &models.VideoFile{}
	if err := v.db.QueryRowxContext(
		ctx,
		createVideoQuery,
		videoFile.UserID,
		videoFile.FileName,
		videoFile.FileSize,
		0,
		videoFile.S3Key,
		videoFile.S3Bucket,
		videoFile.Format,
	).StructScan(video); err != nil {
		return nil, fmt.Errorf("failed to create video: %w", err)
	}
	return video, nil
}

func (v *videoRepo) GetVideos(ctx context.Context, userID uuid.UUID, query *utils.Pagination) (*models.VideoList, error) {
	var totalCount int
	if err := v.db.GetContext(
		ctx,
		&totalCount,
		getTotalVideosByUserIDQuery,
		userID,
	); err != nil {
		return nil, fmt.Errorf("failed to get total videos count: %w", err)
	}
	if totalCount == 0 {
		return &models.VideoList{
			Videos:     make([]*models.VideoFile, 0),
			TotalCount: 0,
			Page:       0,
			PageSize:   0,
			HasMore:    utils.GetHasMore(query.GetPage(), totalCount, query.GetSize()),
		}, nil
	}
	rows, err := v.db.QueryxContext(
		ctx,
		getVideosByUserIDQuery,
		userID,
		query.GetOffset(),
		query.GetLimit(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get videos by user id: %w", err)
	}
	defer rows.Close()
	var videos = make([]*models.VideoFile, 0, query.GetSize())
	for rows.Next() {
		var video models.VideoFile
		if err = rows.StructScan(&video); err != nil {
			return nil, fmt.Errorf("failed to scan video: %w", err)
		}
		videos = append(videos, &video)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to scan videos: %w", err)
	}
	return &models.VideoList{
		Videos:     videos,
		TotalCount: utils.GetTotalPages(totalCount, query.GetSize()),
		Page:       query.GetPage(),
		PageSize:   query.GetSize(),
		HasMore:    utils.GetHasMore(query.GetPage(), totalCount, query.GetSize()),
	}, nil

}

func (v *videoRepo) GetVideoByID(ctx context.Context, videoID uuid.UUID) (*models.VideoFile, error) {
	video := &models.VideoFile{}
	if err := v.db.QueryRowxContext(
		ctx,
		getVideoByIDQuery,
		videoID,
	).StructScan(video); err != nil {
		return nil, fmt.Errorf("failed to get video by id: %w", err)
	}
	return video, nil
}

func (v *videoRepo) UpdateVideo(ctx context.Context, video *models.VideoFile) (*models.VideoFile, error) {
	videoFile := &models.VideoFile{}
	if err := v.db.GetContext(
		ctx,
		videoFile,
		updateVideoQuery,
		&video.FileName,
		&video.FileSize,
		&video.Duration,
		&video.S3Key,
		&video.S3Bucket,
		&video.Format,
		&video.Status,
	); err != nil {
		return nil, fmt.Errorf("failed to update video: %w", err)
	}
	return videoFile, nil
}

func (v *videoRepo) GetVideosByQuery(ctx context.Context, userID uuid.UUID, query string, pq *utils.Pagination) (*models.VideoList, error) {
	var totalCount int
	if err := v.db.GetContext(
		ctx,
		&totalCount,
		getTotalVideosCountQuery,
		userID,
		query,
	); err != nil {
		return nil, fmt.Errorf("failed to get total videos by query: %w", err)
	}
	if totalCount == 0 {
		return &models.VideoList{
			Videos:     make([]*models.VideoFile, 0),
			TotalCount: 0,
			Page:       0,
			PageSize:   0,
			HasMore:    utils.GetHasMore(pq.GetPage(), totalCount, pq.GetSize()),
		}, nil
	}
	rows, err := v.db.QueryxContext(
		ctx,
		getVideosBySearchQuery,
		userID,
		query,
		pq.GetOffset(),
		pq.GetLimit(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get videos by query: %w", err)
	}
	defer rows.Close()
	var videos = make([]*models.VideoFile, 0, pq.GetSize())
	for rows.Next() {
		var video models.VideoFile
		if err = rows.StructScan(&video); err != nil {
			return nil, fmt.Errorf("failed to scan video: %w", err)
		}
		videos = append(videos, &video)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to scan videos: %w", err)
	}
	return &models.VideoList{
		Videos:     videos,
		TotalCount: utils.GetTotalPages(totalCount, pq.GetSize()),
		Page:       pq.GetPage(),
		PageSize:   pq.GetSize(),
		HasMore:    utils.GetHasMore(pq.GetPage(), totalCount, pq.GetSize()),
	}, nil
}

func (v *videoRepo) DeleteVideo(ctx context.Context, userID uuid.UUID, videoID uuid.UUID) error {
	res, err := v.db.ExecContext(
		ctx,
		deleteVideoQuery,
		videoID,
		userID,
	)
	if err != nil {
		return fmt.Errorf("failed to delete video: %w", err)
	}
	count, _ := res.RowsAffected()
	if count == 0 {
		return fmt.Errorf("no video found to delete")
	}
	return nil
}

func (v *videoRepo) GetPlaybackInfo(ctx context.Context, videoID uuid.UUID) (*models.PlaybackInfo, error) {
	playbackInfo := &models.PlaybackInfo{}
	if err := v.db.QueryRowxContext(
		ctx,
		getPlaybackInfoQuery,
		videoID,
	).StructScan(playbackInfo); err != nil {
		return nil, fmt.Errorf("failed to get playback info: %w", err)
	}
	return playbackInfo, nil
}
