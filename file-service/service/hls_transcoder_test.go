package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsVideo(t *testing.T) {
	require.True(t, IsVideo("video/mp4", "clip.bin"))
	require.True(t, IsVideo("application/octet-stream", "clip.mp4"))
	require.True(t, IsVideo("", "movie.mov"))
	require.False(t, IsVideo("image/jpeg", "photo.jpg"))
}

func TestSelectVariants(t *testing.T) {
	v1080 := selectVariants(1080)
	require.Len(t, v1080, 3)

	v480 := selectVariants(480)
	require.Len(t, v480, 2)
	require.Equal(t, "480p", v480[0].name)

	v240 := selectVariants(240)
	require.Len(t, v240, 1)
	require.Equal(t, "360p", v240[0].name)
}

func TestBuildFFmpegHLSArgs(t *testing.T) {
	variants := selectVariants(720)
	args := buildFFmpegHLSArgs("/tmp/in.mp4", "/tmp/out", variants, true)
	require.Contains(t, args, "master.m3u8")
	require.Contains(t, args, "hls")
	require.Contains(t, args, "libx264")
	require.Contains(t, args, "aac")
}
