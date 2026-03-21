package service

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"go-api-server/internal/model"
)

type DownloadResult struct {
	TargetPath        string
	Saved             bool
	StartTime         string
	EndTime           string
	AudioFrameCount   int
	VideoFrameCount   int
	VideoFormat       string
	SelectedOutputFPS int
	ContainerPath     string
	ContainerPaths    []string
	ContainerFormat   string
}

func DownloadToLocalPath(ctx context.Context, req model.DownloadRequest) (DownloadResult, error) {
	result := DownloadResult{
		TargetPath:     req.TargetFolder,
		Saved:          false,
		ContainerPaths: []string{},
	}
	targetAddress := ResolveTargetAddress(req.DeviceIP)
	cfg := GetHostScanCGIConfig()
	runtimeCfg := GetDownloadRuntimeConfig()
	channelIndexes, err := parseChannelIndexes(req.Channel)
	if err != nil {
		return result, err
	}

	infoResult, err := FetchInfoFromTargetWithCredentials(ctx, targetAddress, cfg.Port, cfg.User, cfg.PW)
	if err != nil {
		return result, err
	}
	if len(infoResult) == 0 {
		return result, fmt.Errorf("target info.cgi returned empty response")
	}

	channelHex, err := channelToBitMaskHex(req.Channel)
	if err != nil {
		return result, err
	}

	downloadURL, err := buildDownloadURL(targetAddress, cfg.Port, channelHex, req.Begin, req.End)
	if err != nil {
		return result, err
	}
	if runtimeCfg.Debug {
		fmt.Printf("[download debug] cgi url=%s\n", downloadURL)
	}

	client := &http.Client{Timeout: 60 * time.Second}
	body, statusCode, err := doDigestRequest(ctx, client, http.MethodGet, downloadURL, cfg.User, cfg.PW)
	if err != nil {
		return result, err
	}
	if statusCode < 200 || statusCode >= 300 {
		msg := strings.TrimSpace(string(body))
		if msg == "" {
			msg = "empty error body"
		}
		return result, &TargetHTTPError{
			StatusCode: statusCode,
			Message:    msg,
		}
	}
	if len(body) == 0 {
		return result, fmt.Errorf("download.cgi returned empty body")
	}

	start, end, audioFrameCount, videoFrameCount, videoFormat, outputFPS, _, frames, err := ParseDownloadStreamToRawVideo(body, req.TargetFolder, runtimeCfg.Debug)
	if err != nil {
		return result, err
	}
	sourceFPS := normalizeSourceFPS(cfg.SourceFPS)
	if sourceFPS > 0 {
		outputFPS = sourceFPS
	}
	result.Saved = true
	result.TargetPath = req.TargetFolder
	result.AudioFrameCount = audioFrameCount
	result.VideoFrameCount = videoFrameCount
	result.VideoFormat = videoFormat
	result.SelectedOutputFPS = outputFPS

	if runtimeCfg.JpgOut {
		var wg sync.WaitGroup
		errCh := make(chan error, len(channelIndexes))
		for _, ch := range channelIndexes {
			ch := ch
			wg.Add(1)
			go func() {
				defer wg.Done()
				channelRaw, err := buildRawVideoBytesByChannel(frames, ch)
				if err != nil {
					errCh <- fmt.Errorf("channel %d jpg decode failed: %w", ch, err)
					return
				}
				if err := TranscodeRawBytesToJPGWithMap(channelRaw, req.TargetFolder, outputFPS, ch, runtimeCfg.Debug); err != nil {
					errCh <- fmt.Errorf("channel %d jpg transcode failed: %w", ch, err)
					return
				}
			}()
		}
		wg.Wait()
		close(errCh)
		for err := range errCh {
			if err != nil {
				return result, err
			}
		}
	}
	if runtimeCfg.ContainerOut {
		containerFormat := normalizeContainerFormat(runtimeCfg.ContainerFormat)
		paths := make([]string, len(channelIndexes))
		requestDurationSec := estimateRequestDurationSeconds(req.Begin, req.End)
		var wg sync.WaitGroup
		errCh := make(chan error, len(channelIndexes))
		for i, ch := range channelIndexes {
			i := i
			ch := ch
			wg.Add(1)
			go func() {
				defer wg.Done()
				channelRaw, err := buildRawVideoBytesByChannel(frames, ch)
				if err != nil {
					errCh <- fmt.Errorf("channel %d container build failed: %w", ch, err)
					return
				}
				frameCount := countFramesForChannel(frames, ch)
				muxFPS := chooseMuxInputFPS(frameCount, requestDurationSec, sourceFPS)
				containerPath := buildOutputContainerPathByChannel(req.TargetFolder, req.Begin, req.End, ch, containerFormat)
				if runtimeCfg.Debug {
					fmt.Printf("[container fps] ch=%d frameCount=%d durationSec=%.3f muxInputFPS=%s\n", ch, frameCount, requestDurationSec, formatFFmpegFPS(muxFPS))
				}
				if err := TranscodeRawBytesToContainerWithMap(channelRaw, containerPath, containerFormat, muxFPS, ch, runtimeCfg.Debug); err != nil {
					errCh <- fmt.Errorf("channel %d container transcode failed: %w", ch, err)
					return
				}
				paths[i] = containerPath
			}()
		}
		wg.Wait()
		close(errCh)
		for err := range errCh {
			if err != nil {
				return result, err
			}
		}
		result.ContainerPaths = paths
		if len(paths) > 0 {
			result.ContainerPath = paths[0]
		}
		result.ContainerFormat = containerFormat
	}
	if !start.IsZero() {
		result.StartTime = start.Format("2006-01-02 15:04:05.000")
	}
	if !end.IsZero() {
		result.EndTime = end.Format("2006-01-02 15:04:05.000")
	}
	return result, nil
}

func buildOutputContainerPath(targetFolder, begin, end, channelHex, containerFormat string) string {
	safeChannel := strings.ReplaceAll(strings.ToLower(channelHex), ",", "-")
	fileName := fmt.Sprintf("download_%s_%s_ch%s.%s", begin, end, safeChannel, containerFormat)
	return filepath.Clean(filepath.Join(targetFolder, fileName))
}

func buildOutputContainerPathByChannel(targetFolder, begin, end string, channelIndex int, containerFormat string) string {
	fileName := fmt.Sprintf("download_%s_%s_ch%d.%s", begin, end, channelIndex, containerFormat)
	return filepath.Clean(filepath.Join(targetFolder, fileName))
}

func parseChannelIndexes(channelInput string) ([]int, error) {
	tokens := strings.Split(channelInput, ",")
	out := make([]int, 0, len(tokens))
	seen := make(map[int]struct{})
	for _, token := range tokens {
		trimmed := strings.TrimSpace(token)
		if trimmed == "" {
			continue
		}
		val, err := strconv.Atoi(trimmed)
		if err != nil {
			return nil, fmt.Errorf("invalid channel value: %s", trimmed)
		}
		if val < 1 || val > 8 {
			return nil, fmt.Errorf("channel out of range: %d", val)
		}
		if _, ok := seen[val]; ok {
			continue
		}
		seen[val] = struct{}{}
		out = append(out, val)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("channel is required")
	}
	return out, nil
}

func estimateRequestDurationSeconds(begin, end string) float64 {
	bt, err := parseRequestRangeTime(begin)
	if err != nil {
		return 0
	}
	et, err := parseRequestRangeTime(end)
	if err != nil {
		return 0
	}
	if !et.After(bt) {
		return 0
	}
	return et.Sub(bt).Seconds()
}

func parseRequestRangeTime(value string) (time.Time, error) {
	trimmed := strings.TrimSpace(strings.ReplaceAll(value, "T", ""))
	if len(trimmed) < 14 {
		return time.Time{}, fmt.Errorf("invalid request range time")
	}
	base := trimmed[:14]
	return time.ParseInLocation("20060102150405", base, time.Local)
}

func chooseMuxInputFPS(frameCount int, durationSec float64, fallback int) float64 {
	if fallback >= 1 && fallback <= 120 {
		return float64(fallback)
	}
	if frameCount > 0 && durationSec > 0 {
		derived := float64(frameCount) / durationSec
		if derived >= 1 && derived <= 120 {
			return derived
		}
	}
	return 15
}

func normalizeSourceFPS(v int) int {
	if v >= 1 && v <= 120 {
		return v
	}
	return 0
}

func normalizeContainerFormat(format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "avi":
		return "avi"
	case "mp4", "":
		return "mp4"
	default:
		return "mp4"
	}
}

func buildDownloadURL(ip string, port int, channelHex, begin, end string) (string, error) {
	base := fmt.Sprintf("http://%s/download.cgi", net.JoinHostPort(ip, strconv.Itoa(port)))
	u, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("invalid download url: %w", err)
	}
	beginCGI, err := normalizeDownloadCGITime(begin)
	if err != nil {
		return "", err
	}
	endCGI, err := normalizeDownloadCGITime(end)
	if err != nil {
		return "", err
	}

	query := u.Query()
	query.Set("Channel", channelHex)
	query.Set("Begin", beginCGI)
	query.Set("End", endCGI)
	u.RawQuery = query.Encode()
	return u.String(), nil
}

func normalizeDownloadCGITime(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	trimmed = strings.ReplaceAll(trimmed, "T", "")
	if len(trimmed) < 14 {
		return "", fmt.Errorf("invalid download time format: %s", value)
	}
	base := trimmed
	if len(trimmed) >= 17 {
		base = trimmed[:len(trimmed)-3]
	}
	if len(base) > 14 {
		base = base[:14]
	}
	return base + "000", nil
}

func channelToBitMaskHex(channelInput string) (string, error) {
	tokens := strings.Split(channelInput, ",")
	var mask uint64

	for _, token := range tokens {
		trimmed := strings.TrimSpace(token)
		if trimmed == "" {
			continue
		}

		channelIdx, err := strconv.Atoi(trimmed)
		if err != nil {
			return "", fmt.Errorf("invalid channel value: %s", trimmed)
		}
		if channelIdx < 1 || channelIdx > 64 {
			return "", fmt.Errorf("channel out of range: %d", channelIdx)
		}
		mask |= uint64(1) << uint(channelIdx-1)
	}

	if mask == 0 {
		return "", fmt.Errorf("channel is required")
	}

	hexValue := strings.ToLower(strconv.FormatUint(mask, 16))
	if len(hexValue)%2 != 0 {
		hexValue = "0" + hexValue
	}
	return hexValue, nil
}
