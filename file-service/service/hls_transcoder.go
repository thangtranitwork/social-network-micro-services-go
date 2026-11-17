package service

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/minio/minio-go/v7"
	"social-network-go/logger"
)

const (
	hlsPrefix             = "hls"
	hlsMaxConcurrentJobs  = 2
	hlsTranscodeTimeout   = 15 * time.Minute
	hlsSegmentDurationSec = 6
)

type hlsVariant struct {
	name         string
	height       int
	videoBitrate string
	maxRate      string
	bufSize      string
	audioBitrate string
}

var defaultHLSVariants = []hlsVariant{
	{name: "720p", height: 720, videoBitrate: "2500k", maxRate: "2675k", bufSize: "3750k", audioBitrate: "128k"},
	{name: "480p", height: 480, videoBitrate: "1200k", maxRate: "1284k", bufSize: "1800k", audioBitrate: "96k"},
	{name: "360p", height: 360, videoBitrate: "700k", maxRate: "750k", bufSize: "1050k", audioBitrate: "64k"},
}

// HLSTranscoder converts uploaded videos into multi-bitrate HLS playlists (.m3u8 + .ts) in MinIO.
type HLSTranscoder struct {
	minioClient *minio.Client
	bucket      string
	sem         chan struct{}
	jobs        sync.Map // fileID -> "processing" | "ready" | "failed"
}

func NewHLSTranscoder(minioClient *minio.Client, bucket string) *HLSTranscoder {
	if minioClient == nil || bucket == "" {
		return nil
	}
	return &HLSTranscoder{
		minioClient: minioClient,
		bucket:      bucket,
		sem:         make(chan struct{}, hlsMaxConcurrentJobs),
	}
}

// IsVideo returns true if contentType or filename extension indicates a video file.
func IsVideo(contentType, filename string) bool {
	ct := strings.ToLower(strings.TrimSpace(contentType))
	if strings.HasPrefix(ct, "video/") {
		return true
	}
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".mp4", ".mov", ".webm", ".mkv", ".avi", ".m4v":
		return true
	default:
		return false
	}
}

func hlsObjectKey(fileID, relPath string) string {
	clean := strings.TrimPrefix(filepath.ToSlash(filepath.Clean("/"+relPath)), "/")
	return fmt.Sprintf("%s/%s/%s", hlsPrefix, fileID, clean)
}

func hlsContentType(relPath string) string {
	switch strings.ToLower(filepath.Ext(relPath)) {
	case ".m3u8":
		return "application/vnd.apple.mpegurl"
	case ".ts":
		return "video/MP2T"
	default:
		return "application/octet-stream"
	}
}

// Enqueue schedules asynchronous HLS transcoding for fileID if not already running or ready.
func (t *HLSTranscoder) Enqueue(fileID string) {
	if t == nil || fileID == "" {
		return
	}
	if _, loaded := t.jobs.LoadOrStore(fileID, "processing"); loaded {
		if st, ok := t.jobs.Load(fileID); ok && st != "failed" {
			return
		}
		t.jobs.Store(fileID, "processing")
	}

	go func() {
		t.sem <- struct{}{}
		defer func() { <-t.sem }()

		ctx, cancel := context.WithTimeout(context.Background(), hlsTranscodeTimeout)
		defer cancel()

		if err := t.transcodeToHLS(ctx, fileID); err != nil {
			logger.Err(err).Warn("HLS transcode failed for file %s", fileID)
			t.jobs.Store(fileID, "failed")
			return
		}
		t.jobs.Store(fileID, "ready")
		logger.Info("HLS transcode completed for file %s", fileID)
	}()
}

// Status returns "ready", "processing", "failed", or "not_ready".
func (t *HLSTranscoder) Status(ctx context.Context, fileID string) string {
	if t == nil || fileID == "" {
		return "not_ready"
	}
	if _, err := t.minioClient.StatObject(ctx, t.bucket, hlsObjectKey(fileID, "master.m3u8"), minio.StatObjectOptions{}); err == nil {
		t.jobs.Store(fileID, "ready")
		return "ready"
	}
	if st, ok := t.jobs.Load(fileID); ok {
		if s, ok := st.(string); ok {
			return s
		}
	}
	return "not_ready"
}

// LoadAsset streams a generated HLS playlist or segment from MinIO.
func (t *HLSTranscoder) LoadAsset(ctx context.Context, fileID, relPath string) (io.ReadCloser, string, int64, error) {
	if t == nil {
		return nil, "", 0, fmt.Errorf("hls transcoder not initialized")
	}
	if fileID == "" || relPath == "" || strings.Contains(relPath, "..") {
		return nil, "", 0, fmt.Errorf("invalid hls path")
	}
	key := hlsObjectKey(fileID, relPath)
	info, err := t.minioClient.StatObject(ctx, t.bucket, key, minio.StatObjectOptions{})
	if err != nil {
		return nil, "", 0, fmt.Errorf("hls asset not found: %w", err)
	}
	obj, err := t.minioClient.GetObject(ctx, t.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, "", 0, fmt.Errorf("failed to get hls asset: %w", err)
	}
	return obj, hlsContentType(relPath), info.Size, nil
}

func (t *HLSTranscoder) transcodeToHLS(ctx context.Context, fileID string) error {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return fmt.Errorf("ffmpeg not found in PATH: %w", err)
	}

	tmpDir, err := os.MkdirTemp("", "hls-"+fileID+"-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	srcPath := filepath.Join(tmpDir, "source"+filepath.Ext(fileID))
	if err := t.minioClient.FGetObject(ctx, t.bucket, fileID, srcPath, minio.GetObjectOptions{}); err != nil {
		return fmt.Errorf("download source video: %w", err)
	}

	srcHeight, hasAudio := probeVideo(ctx, srcPath)
	variants := selectVariants(srcHeight)

	outDir := filepath.Join(tmpDir, "out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}

	args := buildFFmpegHLSArgs(srcPath, outDir, variants, hasAudio)
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ffmpeg failed: %w: %s", err, string(output))
	}

	return filepath.WalkDir(outDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() {
			return walkErr
		}
		rel, err := filepath.Rel(outDir, path)
		if err != nil {
			return err
		}
		key := hlsObjectKey(fileID, rel)
		_, err = t.minioClient.FPutObject(ctx, t.bucket, key, path, minio.PutObjectOptions{
			ContentType: hlsContentType(rel),
		})
		return err
	})
}

func probeVideo(ctx context.Context, srcPath string) (height int, hasAudio bool) {
	if _, err := exec.LookPath("ffprobe"); err != nil {
		return 720, true
	}

	vCmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=height",
		"-of", "csv=p=0",
		srcPath,
	)
	if out, err := vCmd.Output(); err == nil {
		if h, convErr := strconv.Atoi(strings.TrimSpace(string(out))); convErr == nil && h > 0 {
			height = h
		}
	}
	if height == 0 {
		height = 720
	}

	aCmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "error",
		"-select_streams", "a:0",
		"-show_entries", "stream=codec_type",
		"-of", "csv=p=0",
		srcPath,
	)
	if out, err := aCmd.Output(); err == nil && strings.TrimSpace(string(out)) != "" {
		hasAudio = true
	}
	return height, hasAudio
}

func selectVariants(srcHeight int) []hlsVariant {
	var selected []hlsVariant
	for _, v := range defaultHLSVariants {
		if v.height <= srcHeight {
			selected = append(selected, v)
		}
	}
	if len(selected) == 0 {
		// Source is smaller than 360p: keep lowest rung as fallback
		selected = []hlsVariant{defaultHLSVariants[len(defaultHLSVariants)-1]}
	}
	return selected
}

func buildFFmpegHLSArgs(srcPath, outDir string, variants []hlsVariant, hasAudio bool) []string {
	args := []string{"-y", "-i", srcPath}

	var filterParts []string
	var streamMaps []string
	for i, v := range variants {
		filterParts = append(filterParts, fmt.Sprintf("[0:v]scale=-2:%d[v%d]", v.height, i))
		args = append(args, "-map", fmt.Sprintf("[v%d]", i))
		if hasAudio {
			args = append(args, "-map", "0:a:0")
			streamMaps = append(streamMaps, fmt.Sprintf("v:%d,a:%d,name:%s", i, i, v.name))
		} else {
			streamMaps = append(streamMaps, fmt.Sprintf("v:%d,name:%s", i, v.name))
		}
	}

	// Prepend filter_complex right after input
	baseArgs := []string{"-y", "-i", srcPath, "-filter_complex", strings.Join(filterParts, ";")}
	mapArgs := args[3:]
	finalArgs := append(baseArgs, mapArgs...)

	for i, v := range variants {
		finalArgs = append(finalArgs,
			fmt.Sprintf("-c:v:%d", i), "libx264",
			fmt.Sprintf("-preset"), "veryfast",
			fmt.Sprintf("-profile:v:%d", i), "main",
			fmt.Sprintf("-b:v:%d", i), v.videoBitrate,
			fmt.Sprintf("-maxrate:v:%d", i), v.maxRate,
			fmt.Sprintf("-bufsize:v:%d", i), v.bufSize,
			"-g", "48",
			"-keyint_min", "48",
			"-sc_threshold", "0",
		)
		if hasAudio {
			finalArgs = append(finalArgs,
				fmt.Sprintf("-c:a:%d", i), "aac",
				fmt.Sprintf("-b:a:%d", i), v.audioBitrate,
				"-ac", "2",
			)
		}
	}

	finalArgs = append(finalArgs,
		"-f", "hls",
		"-hls_time", strconv.Itoa(hlsSegmentDurationSec),
		"-hls_playlist_type", "vod",
		"-hls_flags", "independent_segments",
		"-master_pl_name", "master.m3u8",
		"-var_stream_map", strings.Join(streamMaps, " "),
		"-hls_segment_filename", filepath.Join(outDir, "%v", "seg_%03d.ts"),
		filepath.Join(outDir, "%v", "prog_index.m3u8"),
	)
	return finalArgs
}
