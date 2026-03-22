package service

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	netStreamPacketHeaderSize = 56 // net_stream_t (pack 4) 기준 헤더 크기
	pdvrStreamHeaderSize      = 1144
)

var h264FourCC = map[string]struct{}{
	"H26F": {},
	"H26H": {},
	"H26C": {},
	"H264": {},
	"H26P": {},
	"H26I": {},
	"H26O": {},
	"S26F": {},
	"S269": {},
	"S26H": {},
	"S26C": {},
	"S26S": {},
	"S26U": {},
	"S26V": {},
	"ATH1": {},
}

var videoFourCCSet = map[uint32]struct{}{
	makeFourCC("ADV1"): {}, makeFourCC("MPG2"): {}, makeFourCC("MJPG"): {}, makeFourCC("MJPC"): {},
	makeFourCC("MJPQ"): {}, makeFourCC("mjpq"): {}, makeFourCC("mjpg"): {}, makeFourCC("DNET"): {},
	makeFourCC("MP4C"): {}, makeFourCC("MP4F"): {}, makeFourCC("MP4H"): {}, makeFourCC("MP4P"): {},
	makeFourCC("MPG4"): {}, makeFourCC("H26C"): {}, makeFourCC("H26F"): {}, makeFourCC("H26H"): {},
	makeFourCC("H264"): {}, makeFourCC("H26P"): {}, makeFourCC("H26I"): {}, makeFourCC("H26O"): {},
	makeFourCC("4PMC"): {}, makeFourCC("4PMF"): {}, makeFourCC("4PMH"): {}, makeFourCC("SM4C"): {},
	makeFourCC("SM4F"): {}, makeFourCC("SM4H"): {}, makeFourCC("S26C"): {}, makeFourCC("S26F"): {},
	makeFourCC("S269"): {}, makeFourCC("S26H"): {}, makeFourCC("S26S"): {}, makeFourCC("S26U"): {},
	makeFourCC("S26V"): {}, makeFourCC("IM00"): {}, makeFourCC("IH00"): {}, makeFourCC("ATH1"): {},
	makeFourCC("JPEG"): {}, makeFourCC("MJP0"): {}, makeFourCC("H265"): {}, makeFourCC("SKIP"): {},
}

var audioFourCCSet = map[uint32]struct{}{
	makeFourCC("RA8K"): {}, makeFourCC("MP4A"): {}, makeFourCC("MP4U"): {}, makeFourCC("G721"): {},
	makeFourCC("G726"): {}, makeFourCC("G72A"): {}, makeFourCC("G761"): {}, makeFourCC("G762"): {},
	makeFourCC("G763"): {}, makeFourCC("G764"): {}, makeFourCC("G722"): {}, makeFourCC("GIMA"): {},
	makeFourCC("G723"): {}, makeFourCC("APCM"): {}, makeFourCC("PCM_"): {},
}

type h264Frame struct {
	data        []byte
	keyFrame    bool
	timestamp   time.Time
	tTimeSec    int64
	tTimeUsec   int64
	format      string
	fps         uint32
	channelMask uint8
}

type h264CodecState struct {
	sps []byte
	pps []byte
}

func ParseDownloadStreamToRawVideo(downloadBody []byte, targetFolder string, debug bool, onFrameParsed func(h264Frame)) (time.Time, time.Time, int, int, string, int, []byte, []h264Frame, error) {
	cleanFolder := filepath.Clean(targetFolder)
	if err := os.MkdirAll(cleanFolder, 0o755); err != nil {
		return time.Time{}, time.Time{}, 0, 0, "", 0, nil, nil, fmt.Errorf("failed to create target folder: %w", err)
	}

	frames, _, _, audioFrameCount, _, err := extractH264Frames(downloadBody, debug, cleanFolder, onFrameParsed)
	if err != nil {
		return time.Time{}, time.Time{}, 0, 0, "", 0, nil, nil, err
	}
	if len(frames) == 0 {
		return time.Time{}, time.Time{}, audioFrameCount, 0, "", 0, nil, nil, fmt.Errorf("no h264 frames found in download stream")
	}

	videoFrameCount := len(frames)
	videoFormat := frames[0].format
	outputFPS := decideOutputFPS(frames)

	rawVideo, err := buildRawVideoBytes(frames)
	if err != nil {
		return time.Time{}, time.Time{}, audioFrameCount, 0, "", 0, nil, nil, err
	}
	start, end := findTimeRange(frames)
	return start, end, audioFrameCount, videoFrameCount, videoFormat, outputFPS, rawVideo, frames, nil
}

func extractH264Frames(stream []byte, debug bool, targetFolder string, onFrameParsed func(h264Frame)) ([]h264Frame, time.Time, time.Time, int, int, error) {
	if len(stream) == 0 {
		return nil, time.Time{}, time.Time{}, 0, 0, fmt.Errorf("empty stream data")
	}

	offset := 0
	if len(stream) >= 8 && string(stream[:4]) == "PDVR" {
		// ClipDownload.cpp에서 net_stream_header_t.cbSize는 ntohl로 읽기 때문에 big-endian으로 해석한다.
		headerSize := int(binary.BigEndian.Uint32(stream[4:8]))
		if headerSize > 0 && headerSize <= len(stream) {
			offset = headerSize
		} else if len(stream) >= pdvrStreamHeaderSize {
			offset = pdvrStreamHeaderSize
		}
	}

	frames := make([]h264Frame, 0)
	var start, end time.Time
	fourccCount := make(map[string]int)
	audioFrameCount := 0
	videoFrameCount := 0
	codecState := h264CodecState{}

	for offset+4 <= len(stream) {
		if offset+4 > len(stream) {
			break
		}
		sync := binary.LittleEndian.Uint32(stream[offset : offset+4])

		if sync == makeFourCC("PDVR") {
			if offset+8 > len(stream) {
				break
			}
			headerSize := int(binary.BigEndian.Uint32(stream[offset+4 : offset+8]))
			if headerSize <= 0 || offset+headerSize > len(stream) {
				offset++
				continue
			}
			offset += headerSize
			continue
		}

		if sync == 0xFFFFFFFF {
			// wavelet_stream_t: dwSize는 network order이며, 원본 코드 기준 packet total size로 동작한다.
			if offset+12 > len(stream) {
				break
			}
			advSize := int(binary.BigEndian.Uint32(stream[offset+8 : offset+12]))
			if advSize <= 0 || offset+advSize > len(stream) {
				offset++
				continue
			}
			offset += advSize
			continue
		}

		if isAnyCarData(sync) {
			if offset+8 > len(stream) {
				break
			}
			carSize := int(int32(binary.LittleEndian.Uint32(stream[offset+4 : offset+8])))
			totalSize := 8 + carSize
			if carSize < 0 || offset+totalSize > len(stream) {
				offset++
				continue
			}
			offset += totalSize
			continue
		}

		_, isVideoPacket := videoFourCCSet[sync]
		_, isAudioPacket := audioFourCCSet[sync]
		if !isVideoPacket && !isAudioPacket {
			offset++
			continue
		}
		if offset+netStreamPacketHeaderSize > len(stream) {
			break
		}

		header := stream[offset : offset+netStreamPacketHeaderSize]
		payloadSize, ok := parsePayloadSize(header, len(stream)-offset-netStreamPacketHeaderSize)
		if !ok {
			offset++
			continue
		}
		totalSize := netStreamPacketHeaderSize + payloadSize
		tag := string(header[:4])
		fourccCount[tag]++
		if isAudioPacket {
			// download API는 오디오를 처리하지 않고 패킷만 소비한다.
			audioFrameCount++
			offset += totalSize
			continue
		}

		flags := binary.LittleEndian.Uint32(header[12:16])
		frameType := (flags >> 1) & 0x3
		cbSize := binary.LittleEndian.Uint32(header[4:8])
		channelAndFPS := binary.LittleEndian.Uint32(header[16:20])
		nChannelRaw := selectChannelMask(channelAndFPS)
		nChannel := channelMaskToIndexString(nChannelRaw)
		nFPS := (channelAndFPS >> 24) & 0xFF
		nDTS := readPacketNDTS(header)
		secRaw, usecRaw := readPacketTimeParts(header)
		frameTime := readPacketTime(header)
		if debug {
			fmt.Printf("[video frame] nChannel=%s fourcc=%s nType=%d nFPS=%d cbSize=%d tTime=%d.%06d nDTS=%d\n",
				nChannel, tag, frameType, nFPS, cbSize, secRaw, usecRaw, nDTS)
		}
		payload := stream[offset+netStreamPacketHeaderSize : offset+totalSize]
		if len(payload) > 0 {
			updateCodecState(&codecState, payload)
		}
		if _, isH264 := h264FourCC[tag]; isH264 && len(payload) > 0 {
			if start.IsZero() && !frameTime.IsZero() {
				start = frameTime
			}
			if !frameTime.IsZero() {
				end = frameTime
			}
			frames = append(frames, h264Frame{
				data:        slices.Clone(payload),
				keyFrame:    frameType == 0,
				timestamp:   frameTime,
				tTimeSec:    secRaw,
				tTimeUsec:   usecRaw,
				format:      sanitizeFourCC(tag),
				fps:         nFPS,
				channelMask: nChannelRaw,
			})
			if onFrameParsed != nil {
				onFrameParsed(frames[len(frames)-1])
			}
			videoFrameCount++
		}
		offset += totalSize
	}

	if len(frames) == 0 {
		return nil, time.Time{}, time.Time{}, audioFrameCount, videoFrameCount, fmt.Errorf("no h264 frames found in download stream (found fourcc: %s)", summarizeFourCC(fourccCount))
	}

	return frames, start, end, audioFrameCount, videoFrameCount, nil
}

func decodeFramesToJPG(targetFolder string, frames []h264Frame, fps int, debug bool) error {
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		fmt.Println("[download warn] ffmpeg not found in PATH; skip jpg decoding")
		return nil
	}

	var stream bytes.Buffer
	state := h264CodecState{}
	normalizedCount := 0
	skippedCount := 0
	prefixedHeader := false
	for _, frame := range frames {
		annexB, ok := normalizeToAnnexB(frame.data)
		if !ok || len(annexB) == 0 {
			skippedCount++
			continue
		}
		updateCodecState(&state, annexB)
		// 프레임마다 SPS/PPS를 반복 주입하면 디코더가 과도한 복원을 수행할 수 있어,
		// 연속 스트림 재인코딩 시에는 초기 1회만 헤더를 붙인다.
		if !prefixedHeader {
			if len(state.sps) > 0 {
				_, _ = stream.Write([]byte{0x00, 0x00, 0x00, 0x01})
				_, _ = stream.Write(state.sps)
			}
			if len(state.pps) > 0 {
				_, _ = stream.Write([]byte{0x00, 0x00, 0x00, 0x01})
				_, _ = stream.Write(state.pps)
			}
			prefixedHeader = true
		}
		_, _ = stream.Write(annexB)
		normalizedCount++
	}

	if debug {
		fmt.Printf("[jpg analyze] totalFrames=%d normalized=%d skipped=%d\n", len(frames), normalizedCount, skippedCount)
	}
	if normalizedCount == 0 || stream.Len() == 0 {
		return fmt.Errorf("no decodable frames for jpg conversion")
	}

	tmpIn, err := os.CreateTemp(targetFolder, "all-frames-*.h264")
	if err != nil {
		return fmt.Errorf("failed to create temp stream for jpg decode: %w", err)
	}
	tmpInputPath := tmpIn.Name()
	if _, err := tmpIn.Write(stream.Bytes()); err != nil {
		_ = tmpIn.Close()
		_ = os.Remove(tmpInputPath)
		return fmt.Errorf("failed to write temp h264 stream: %w", err)
	}
	_ = tmpIn.Close()
	defer func() {
		_ = os.Remove(tmpInputPath)
	}()

	outputPattern := filepath.Join(targetFolder, "%d.jpg")
	cmd := exec.Command(ffmpegPath, "-loglevel", "error", "-y", "-framerate", strconv.Itoa(fps), "-f", "h264", "-i", tmpInputPath, "-vsync", "0", "-start_number", "1", outputPattern)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to decode frames to jpg: %w, output=%s", err, strings.TrimSpace(string(output)))
	}
	if debug {
		fmt.Printf("[jpg] decoded_to=%s\n", outputPattern)
	}
	return nil
}

func buildRawVideoBytes(frames []h264Frame) ([]byte, error) {
	var buf bytes.Buffer
	state := h264CodecState{}
	normalizedCount := 0
	started := false
	for _, frame := range frames {
		if len(frame.data) == 0 {
			continue
		}
		// 컨테이너 copy 입력에서는 과도한 헤더 스킵 복구를 피하고,
		// 보수적 정규화만 사용해 오검출된 SPS/PPS 주입을 줄인다.
		annexB, ok := normalizeToAnnexBForMux(frame.data)
		if !ok || len(annexB) == 0 {
			// copy mux 안정성을 위해 정규화 실패 payload는 제외한다.
			continue
		}

		updateCodecState(&state, annexB)
		hasSPS, hasPPS := containsSPSPPS(annexB)
		isIDR := containsIDRAnnexB(annexB)

		// 시작 프레임 드롭을 줄여 요청 구간 길이를 최대한 보전한다.
		if !started {
			if len(state.sps) > 0 && len(state.pps) > 0 {
				_, _ = buf.Write([]byte{0x00, 0x00, 0x00, 0x01})
				_, _ = buf.Write(state.sps)
				_, _ = buf.Write([]byte{0x00, 0x00, 0x00, 0x01})
				_, _ = buf.Write(state.pps)
			}
			started = true
		}

		if isIDR {
			// IDR 앞에 SPS/PPS가 없는 경우 캐시된 헤더를 보강해서 디코더 안정성을 높인다.
			if !hasSPS && len(state.sps) > 0 {
				_, _ = buf.Write([]byte{0x00, 0x00, 0x00, 0x01})
				_, _ = buf.Write(state.sps)
			}
			if !hasPPS && len(state.pps) > 0 {
				_, _ = buf.Write([]byte{0x00, 0x00, 0x00, 0x01})
				_, _ = buf.Write(state.pps)
			}
		}

		_, _ = buf.Write(annexB)
		normalizedCount++
	}
	if buf.Len() == 0 || normalizedCount == 0 {
		return nil, fmt.Errorf("no decodable h264 payload to transcode")
	}
	return buf.Bytes(), nil
}

func containsSPSPPS(annexB []byte) (bool, bool) {
	hasSPS := false
	hasPPS := false
	for _, nalu := range splitAnnexBNALUs(annexB) {
		if len(nalu) == 0 {
			continue
		}
		switch nalu[0] & 0x1F {
		case 7:
			hasSPS = true
		case 8:
			hasPPS = true
		}
	}
	return hasSPS, hasPPS
}

func containsIDRAnnexB(annexB []byte) bool {
	for _, nalu := range splitAnnexBNALUs(annexB) {
		if len(nalu) == 0 {
			continue
		}
		if nalu[0]&0x1F == 5 {
			return true
		}
	}
	return false
}

func buildRawVideoBytesByChannel(frames []h264Frame, channelIndex int) ([]byte, error) {
	var filtered []h264Frame
	bit := uint8(1 << uint(channelIndex-1))
	for _, frame := range frames {
		if frame.channelMask&bit != 0 {
			filtered = append(filtered, frame)
			continue
		}
		// 일부 장비는 SPS/PPS를 channelMask=0으로 보내므로, 헤더 프레임은 채널 스트림에 보강 포함한다.
		if frame.channelMask == 0 && frameContainsParameterSet(frame.data) {
			filtered = append(filtered, frame)
		}
	}
	if len(filtered) == 0 {
		return nil, fmt.Errorf("no frames found for channel %d", channelIndex)
	}
	return buildRawVideoBytes(filtered)
}

func frameContainsParameterSet(payload []byte) bool {
	annexB, ok := normalizeToAnnexBForMux(payload)
	if !ok || len(annexB) == 0 {
		return false
	}
	hasSPS, hasPPS := containsSPSPPS(annexB)
	return hasSPS || hasPPS
}

func countFramesForChannel(frames []h264Frame, channelIndex int) int {
	bit := uint8(1 << uint(channelIndex-1))
	count := 0
	for _, frame := range frames {
		if frame.channelMask&bit != 0 {
			count++
		}
	}
	return count
}

func normalizeToAnnexBForMux(payload []byte) ([]byte, bool) {
	if len(payload) < 2 {
		return nil, false
	}
	// 1) 표준/준표준 형식 우선
	if data, ok := normalizeWithoutCustomHeader(payload); ok {
		return data, true
	}
	// 2) copy mux 경로는 공격적 custom-header skip을 사용하지 않는다.
	return nil, false
}

func selectChannelMask(channelAndFPS uint32) uint8 {
	b0 := uint8(channelAndFPS & 0xFF)
	b1 := uint8((channelAndFPS >> 8) & 0xFF)
	b2 := uint8((channelAndFPS >> 16) & 0xFF)

	candidates := []uint8{b2, b1, b0}

	// 우선순위 1: 단일 채널(bit 1개) 마스크
	for _, c := range candidates {
		if c != 0 && isSingleBitMask(c) {
			return c
		}
	}
	// 우선순위 2: 다중 비트 채널 마스크
	for _, c := range candidates {
		if c != 0 {
			return c
		}
	}
	return 0
}

func isSingleBitMask(v uint8) bool {
	return v != 0 && (v&(v-1)) == 0
}

func TranscodeRawToContainer(rawPath, outputPath, format string, fps float64, debug bool) error {
	fpsArg := formatFFmpegFPS(fps)
	setTSArg := fmt.Sprintf("setts=pts=N/(%s*TB):dts=N/(%s*TB)", fpsArg, fpsArg)
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		return fmt.Errorf("failed to transcode: ffmpeg not found in PATH")
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return fmt.Errorf("failed to create container output folder: %w", err)
	}

	var cmd *exec.Cmd
	switch strings.ToLower(format) {
	case "avi":
		cmd = exec.Command(ffmpegPath, "-loglevel", "error", "-y", "-framerate", fpsArg, "-f", "h264", "-analyzeduration", "0", "-probesize", "32", "-fflags", "+genpts", "-i", rawPath, "-map", "0:v:0", "-c:v", "copy", "-bsf:v", setTSArg, "-fps_mode", "cfr", "-r", fpsArg, outputPath)
	case "mp4":
		cmd = exec.Command(ffmpegPath, "-loglevel", "error", "-y", "-framerate", fpsArg, "-f", "h264", "-analyzeduration", "0", "-probesize", "32", "-fflags", "+genpts", "-i", rawPath, "-map", "0:v:0", "-c:v", "copy", "-bsf:v", setTSArg, "-fps_mode", "cfr", "-r", fpsArg, "-movflags", "+faststart", outputPath)
	default:
		return fmt.Errorf("unsupported container format: %s", format)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to transcode raw video to %s: %w, output=%s", format, err, strings.TrimSpace(string(output)))
	}
	if debug {
		fmt.Printf("[container] format=%s output=%s\n", strings.ToLower(format), outputPath)
	}
	return nil
}

func TranscodeRawBytesToContainer(rawVideo []byte, outputPath, format string, fps float64, debug bool) error {
	if len(rawVideo) == 0 {
		return fmt.Errorf("failed to transcode: empty raw video payload")
	}
	tmpFile, err := os.CreateTemp("", "download-*.h264")
	if err != nil {
		return fmt.Errorf("failed to create temp raw stream: %w", err)
	}
	tmpPath := tmpFile.Name()
	if _, err := tmpFile.Write(rawVideo); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to write temp raw stream: %w", err)
	}
	_ = tmpFile.Close()
	defer func() {
		_ = os.Remove(tmpPath)
	}()
	return TranscodeRawToContainer(tmpPath, outputPath, format, fps, debug)
}

func TranscodeRawBytesToContainerWithMap(rawVideo []byte, outputPath, format string, fps float64, channelIndex int, debug bool, onWriteProgress func(int)) error {
	if len(rawVideo) == 0 {
		return fmt.Errorf("failed to transcode: empty raw video payload")
	}
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		return fmt.Errorf("failed to transcode: ffmpeg not found in PATH")
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return fmt.Errorf("failed to create container output folder: %w", err)
	}

	fpsArg := formatFFmpegFPS(fps)
	setTSArg := fmt.Sprintf("setts=pts=N/(%s*TB):dts=N/(%s*TB)", fpsArg, fpsArg)

	var cmd *exec.Cmd
	switch strings.ToLower(format) {
	case "avi":
		cmd = exec.Command(ffmpegPath, "-loglevel", "error", "-y", "-framerate", fpsArg, "-f", "h264", "-analyzeduration", "0", "-probesize", "32", "-fflags", "+genpts", "-i", "pipe:0", "-map", "0:v:0", "-c:v", "copy", "-bsf:v", setTSArg, "-fps_mode", "cfr", "-r", fpsArg, outputPath)
	case "mp4":
		cmd = exec.Command(ffmpegPath, "-loglevel", "error", "-y", "-framerate", fpsArg, "-f", "h264", "-analyzeduration", "0", "-probesize", "32", "-fflags", "+genpts", "-i", "pipe:0", "-map", "0:v:0", "-c:v", "copy", "-bsf:v", setTSArg, "-fps_mode", "cfr", "-r", fpsArg, "-movflags", "+faststart", outputPath)
	default:
		return fmt.Errorf("unsupported container format: %s", format)
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create ffmpeg stdin pipe for channel %d: %w", channelIndex, err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start ffmpeg for channel %d: %w", channelIndex, err)
	}
	const writeChunkSize = 64 * 1024
	for offset := 0; offset < len(rawVideo); {
		end := offset + writeChunkSize
		if end > len(rawVideo) {
			end = len(rawVideo)
		}
		written, err := stdin.Write(rawVideo[offset:end])
		if written > 0 && onWriteProgress != nil {
			onWriteProgress(written)
		}
		if err != nil {
			_ = stdin.Close()
			_ = cmd.Wait()
			return fmt.Errorf("failed to write channel %d raw stream to ffmpeg pipe: %w", channelIndex, err)
		}
		if written <= 0 {
			_ = stdin.Close()
			_ = cmd.Wait()
			return fmt.Errorf("failed to write channel %d raw stream to ffmpeg pipe: zero bytes written", channelIndex)
		}
		offset += written
	}
	_ = stdin.Close()
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("failed to transcode channel %d raw video to %s: %w, output=%s", channelIndex, format, err, strings.TrimSpace(stderr.String()))
	}
	if debug {
		fmt.Printf("[container] channel=%d map=0:v:0 input=pipe:0 format=%s output=%s\n", channelIndex, strings.ToLower(format), outputPath)
	}
	return nil
}

func TranscodeRawBytesToJPGWithMap(rawVideo []byte, targetFolder string, fps int, channelIndex int, debug bool) error {
	if len(rawVideo) == 0 {
		return fmt.Errorf("failed to decode jpg: empty raw video payload")
	}
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		fmt.Println("[download warn] ffmpeg not found in PATH; skip jpg decoding")
		return nil
	}
	if err := os.MkdirAll(targetFolder, 0o755); err != nil {
		return fmt.Errorf("failed to create jpg output folder: %w", err)
	}
	if fps < 5 || fps > 120 {
		fps = 15
	}

	outputPattern := filepath.Join(targetFolder, fmt.Sprintf("%%d_ch%d.jpg", channelIndex))
	cmd := exec.Command(ffmpegPath, "-loglevel", "error", "-y", "-framerate", strconv.Itoa(fps), "-f", "h264", "-i", "pipe:0", "-map", "0:v:0", "-vsync", "0", "-start_number", "1", outputPattern)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create ffmpeg stdin pipe for jpg channel %d: %w", channelIndex, err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start ffmpeg for jpg channel %d: %w", channelIndex, err)
	}
	if _, err := stdin.Write(rawVideo); err != nil {
		_ = stdin.Close()
		_ = cmd.Wait()
		return fmt.Errorf("failed to write channel %d jpg raw stream to ffmpeg pipe: %w", channelIndex, err)
	}
	_ = stdin.Close()
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("failed to decode channel %d frames to jpg: %w, output=%s", channelIndex, err, strings.TrimSpace(stderr.String()))
	}
	if debug {
		fmt.Printf("[jpg] channel=%d map=0:v:0 input=pipe:0 decoded_to=%s\n", channelIndex, outputPattern)
	}
	return nil
}

func decideOutputFPS(frames []h264Frame) int {
	freq := make(map[uint32]int)
	for _, frame := range frames {
		if frame.fps >= 5 && frame.fps <= 120 {
			freq[frame.fps]++
		}
	}
	var selected uint32
	maxCount := 0
	for fps, count := range freq {
		if count > maxCount {
			maxCount = count
			selected = fps
		}
	}
	if selected > 0 {
		return int(selected)
	}

	estimated := estimateFPS(frames)
	if estimated >= 5 && estimated <= 120 {
		return estimated
	}
	// 장비 기본값 fallback
	return 15
}

func findTimeRange(frames []h264Frame) (time.Time, time.Time) {
	var start, end time.Time
	for _, frame := range frames {
		if frame.timestamp.IsZero() {
			continue
		}
		if start.IsZero() {
			start = frame.timestamp
		}
		end = frame.timestamp
	}
	return start, end
}

func parsePayloadSize(header []byte, remaining int) (int, bool) {
	// net_stream_t.nSize 위치는 [52:56]
	sizeLE := int(int32(binary.LittleEndian.Uint32(header[52:56])))
	if sizeLE >= 0 && sizeLE <= remaining {
		return sizeLE, true
	}
	sizeBE := int(int32(binary.BigEndian.Uint32(header[52:56])))
	if sizeBE >= 0 && sizeBE <= remaining {
		return sizeBE, true
	}
	// 구버전/변형 헤더 호환: [56:60] 위치도 fallback 시도
	if len(header) >= 60 {
		sizeLEAlt := int(int32(binary.LittleEndian.Uint32(header[56:60])))
		if sizeLEAlt >= 0 && sizeLEAlt <= remaining {
			return sizeLEAlt, true
		}
		sizeBEAlt := int(int32(binary.BigEndian.Uint32(header[56:60])))
		if sizeBEAlt >= 0 && sizeBEAlt <= remaining {
			return sizeBEAlt, true
		}
	}
	return 0, false
}

func makeFourCC(tag string) uint32 {
	if len(tag) != 4 {
		return 0
	}
	return uint32(tag[0]) | uint32(tag[1])<<8 | uint32(tag[2])<<16 | uint32(tag[3])<<24
}

func isAnyCarData(sync uint32) bool {
	evn := uint32('E') | uint32('V')<<8 | uint32('N')<<16
	if (sync & 0x00FFFFFF) != evn {
		return false
	}
	last := byte((sync >> 24) & 0xFF)
	return last >= 'A' && last <= 'Z'
}

func summarizeFourCC(counts map[string]int) string {
	if len(counts) == 0 {
		return "none"
	}
	type entry struct {
		tag   string
		count int
	}
	entries := make([]entry, 0, len(counts))
	for tag, count := range counts {
		entries = append(entries, entry{tag: tag, count: count})
	}
	slices.SortFunc(entries, func(a, b entry) int {
		if a.count != b.count {
			return b.count - a.count
		}
		return strings.Compare(a.tag, b.tag)
	})
	if len(entries) > 8 {
		entries = entries[:8]
	}
	parts := make([]string, 0, len(entries))
	for _, item := range entries {
		parts = append(parts, fmt.Sprintf("%s(%s)", sanitizeFourCC(item.tag), strconv.Itoa(item.count)))
	}
	return strings.Join(parts, ", ")
}

func sanitizeFourCC(tag string) string {
	if len(tag) != 4 {
		return "UNKN"
	}
	out := make([]byte, 4)
	for i := 0; i < 4; i++ {
		c := tag[i]
		if c >= 'a' && c <= 'z' {
			out[i] = c - ('a' - 'A')
			continue
		}
		if c >= 'A' && c <= 'Z' {
			out[i] = c
			continue
		}
		out[i] = 'X'
	}
	return string(out)
}

func readPacketTimeParts(header []byte) (int64, int64) {
	// net_stream_t.tTime (timeval) 위치: [20:28] (sec/usec)
	sec := int64(int32(binary.LittleEndian.Uint32(header[20:24])))
	usec := int64(int32(binary.LittleEndian.Uint32(header[24:28])))
	if usec < 0 || usec >= 1_000_000 {
		usec = 0
	}
	return sec, usec
}

func readPacketTime(header []byte) time.Time {
	sec64, usec64 := readPacketTimeParts(header)
	// 2017-01-01 ~ 2100-01-01 범위를 벗어나면 장비 타임스탬프 손상으로 간주한다.
	if sec64 < 1483228800 || sec64 > 4102444800 {
		return time.Time{}
	}
	return time.Unix(sec64, usec64*1000).Local()
}

func formatFFmpegFPS(fps float64) string {
	if fps < 1 || fps > 120 {
		return "15"
	}
	s := fmt.Sprintf("%.6f", fps)
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	if s == "" {
		return "15"
	}
	return s
}

func readPacketNDTS(header []byte) int64 {
	// net_stream_t.nDTS 위치: [36:44]
	return int64(binary.LittleEndian.Uint64(header[36:44]))
}

func channelMaskToIndexString(mask uint8) string {
	if mask == 0 {
		return "0"
	}
	channels := make([]string, 0, 8)
	for bit := 0; bit < 8; bit++ {
		if mask&(1<<bit) != 0 {
			channels = append(channels, strconv.Itoa(bit+1))
		}
	}
	return strings.Join(channels, ",")
}

func channelMaskToIndexes(mask uint8) []int {
	if mask == 0 {
		return []int{0}
	}
	indexes := make([]int, 0, 8)
	for bit := 0; bit < 8; bit++ {
		if mask&(1<<bit) != 0 {
			indexes = append(indexes, bit+1)
		}
	}
	return indexes
}

func saveIFrameJPG(targetFolder string, payload []byte, globalIndex *int, codecState h264CodecState) (bool, string, error) {
	*globalIndex = *globalIndex + 1
	fileName := fmt.Sprintf("%d.jpg", *globalIndex)
	fullPath := filepath.Join(targetFolder, fileName)

	if isJPEGPayload(payload) {
		if err := os.WriteFile(fullPath, payload, 0o644); err != nil {
			return false, "", fmt.Errorf("failed to save jpeg i-frame: %w", err)
		}
		return true, fullPath, nil
	}

	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		return false, "", nil
	}

	annexB, ok := buildDecodeInput(payload, codecState)
	if !ok || len(annexB) == 0 {
		return false, "", fmt.Errorf("payload cannot be normalized to decodable h264")
	}
	tmpIn, err := os.CreateTemp(targetFolder, "iframe-*.h264")
	if err != nil {
		return false, "", fmt.Errorf("failed to create temp input for jpg decode: %w", err)
	}
	tmpInputPath := tmpIn.Name()
	if _, err := tmpIn.Write(annexB); err != nil {
		_ = tmpIn.Close()
		_ = os.Remove(tmpInputPath)
		return false, "", fmt.Errorf("failed to write temp h264 frame: %w", err)
	}
	_ = tmpIn.Close()
	defer func() {
		_ = os.Remove(tmpInputPath)
	}()

	cmd := exec.Command(ffmpegPath, "-loglevel", "error", "-y", "-f", "h264", "-i", tmpInputPath, "-frames:v", "1", fullPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false, "", fmt.Errorf("failed to decode i-frame to jpg: %w, output=%s", err, strings.TrimSpace(string(output)))
	}
	return true, fullPath, nil
}

func isJPEGPayload(payload []byte) bool {
	return len(payload) >= 4 && payload[0] == 0xFF && payload[1] == 0xD8 && payload[len(payload)-2] == 0xFF && payload[len(payload)-1] == 0xD9
}

func ensureAnnexB(payload []byte) []byte {
	annexB, ok := normalizeToAnnexB(payload)
	if ok {
		return annexB
	}
	out := make([]byte, 4+len(payload))
	copy(out[:4], []byte{0x00, 0x00, 0x00, 0x01})
	copy(out[4:], payload)
	return out
}

func updateCodecState(state *h264CodecState, payload []byte) {
	annexB, ok := normalizeToAnnexB(payload)
	if !ok {
		return
	}
	for _, nalu := range splitAnnexBNALUs(annexB) {
		if len(nalu) == 0 {
			continue
		}
		nalType := nalu[0] & 0x1F
		if nalType == 7 {
			state.sps = slices.Clone(nalu)
		}
		if nalType == 8 {
			state.pps = slices.Clone(nalu)
		}
	}
}

func buildDecodeInput(payload []byte, state h264CodecState) ([]byte, bool) {
	annexB, ok := normalizeToAnnexB(payload)
	if !ok {
		return nil, false
	}
	hasSPS := false
	hasPPS := false
	for _, nalu := range splitAnnexBNALUs(annexB) {
		if len(nalu) == 0 {
			continue
		}
		switch nalu[0] & 0x1F {
		case 7:
			hasSPS = true
		case 8:
			hasPPS = true
		}
	}
	if hasSPS && hasPPS {
		return annexB, true
	}
	var out bytes.Buffer
	if !hasSPS && len(state.sps) > 0 {
		_, _ = out.Write([]byte{0x00, 0x00, 0x00, 0x01})
		_, _ = out.Write(state.sps)
	}
	if !hasPPS && len(state.pps) > 0 {
		_, _ = out.Write([]byte{0x00, 0x00, 0x00, 0x01})
		_, _ = out.Write(state.pps)
	}
	_, _ = out.Write(annexB)
	return out.Bytes(), true
}

func normalizeToAnnexB(payload []byte) ([]byte, bool) {
	if len(payload) < 2 {
		return nil, false
	}

	// 1) 기본 정규화 시도
	if data, ok := normalizeWithoutCustomHeader(payload); ok {
		return data, true
	}

	// 2) 앞단 커스텀 헤더 길이 자동 추정(공격적 복구)
	if data, ok := recoverWithCustomHeaderSkip(payload); ok {
		return data, true
	}
	return nil, false
}

func normalizeWithoutCustomHeader(payload []byte) ([]byte, bool) {
	// Annex-B start code가 중간에 있어도 그 지점부터 정규화
	start := findAnnexBStart(payload)
	if start >= 0 {
		data := payload[start:]
		nalus := splitAnnexBNALUs(data)
		if len(nalus) > 0 {
			valid := 0
			for _, nalu := range nalus {
				if len(nalu) == 0 {
					continue
				}
				nalType := nalu[0] & 0x1F
				if nalType > 0 && nalType <= 23 {
					valid++
				}
			}
			if valid > 0 {
				return data, true
			}
		}
	}

	// AVCC(length-prefixed) 프레임 시도
	if data, ok := convertAVCCToAnnexB(payload); ok {
		return data, true
	}

	// 일부 장비는 raw NAL payload(길이필드/시작코드 없음)를 보낼 수 있어 헤더 오프셋을 휴리스틱으로 탐색
	if data, ok := wrapRawNALAsAnnexB(payload); ok {
		return data, true
	}
	return nil, false
}

func recoverWithCustomHeaderSkip(payload []byte) ([]byte, bool) {
	maxSkip := 512
	if len(payload)-1 < maxSkip {
		maxSkip = len(payload) - 1
	}
	bestScore := -1
	var best []byte
	for skip := 1; skip <= maxSkip; skip++ {
		candidate := payload[skip:]
		if len(candidate) < 8 {
			continue
		}
		data, ok := normalizeWithoutCustomHeader(candidate)
		if !ok {
			continue
		}
		score := scoreAnnexBStream(data)
		if score > bestScore {
			bestScore = score
			best = data
		}
	}
	if bestScore >= 3 && len(best) > 0 {
		return best, true
	}
	return nil, false
}

func findAnnexBStart(payload []byte) int {
	for i := 0; i+3 < len(payload); i++ {
		if payload[i] == 0x00 && payload[i+1] == 0x00 && payload[i+2] == 0x01 {
			return i
		}
		if i+4 < len(payload) && payload[i] == 0x00 && payload[i+1] == 0x00 && payload[i+2] == 0x00 && payload[i+3] == 0x01 {
			return i
		}
	}
	return -1
}

func convertAVCCToAnnexB(payload []byte) ([]byte, bool) {
	if len(payload) < 8 {
		return nil, false
	}
	var out bytes.Buffer
	pos := 0
	validNAL := 0
	for pos+4 <= len(payload) {
		nalLen := int(binary.BigEndian.Uint32(payload[pos : pos+4]))
		pos += 4
		if nalLen <= 0 || pos+nalLen > len(payload) {
			return nil, false
		}
		nalu := payload[pos : pos+nalLen]
		pos += nalLen
		nalType := nalu[0] & 0x1F
		if nalType == 0 || nalType > 23 {
			return nil, false
		}
		_, _ = out.Write([]byte{0x00, 0x00, 0x00, 0x01})
		_, _ = out.Write(nalu)
		validNAL++
	}
	if validNAL == 0 {
		return nil, false
	}
	return out.Bytes(), true
}

func scoreAnnexBStream(data []byte) int {
	nalus := splitAnnexBNALUs(data)
	if len(nalus) == 0 {
		return 0
	}
	score := 0
	for _, nalu := range nalus {
		if len(nalu) == 0 {
			continue
		}
		nalType := nalu[0] & 0x1F
		if nalType == 0 || nalType > 23 {
			continue
		}
		score++
		if nalType == 7 || nalType == 8 || nalType == 5 {
			score += 2
		}
	}
	return score
}

func wrapRawNALAsAnnexB(payload []byte) ([]byte, bool) {
	maxOffset := 32
	if len(payload)-1 < maxOffset {
		maxOffset = len(payload) - 1
	}
	for offset := 0; offset <= maxOffset; offset++ {
		if !isLikelyH264NALHeader(payload[offset]) {
			continue
		}
		data := payload[offset:]
		if len(data) < 4 {
			continue
		}
		out := make([]byte, 4+len(data))
		copy(out[:4], []byte{0x00, 0x00, 0x00, 0x01})
		copy(out[4:], data)
		return out, true
	}
	return nil, false
}

func isLikelyH264NALHeader(b byte) bool {
	nalType := b & 0x1F
	nri := (b >> 5) & 0x03
	if nalType == 0 || nalType > 23 {
		return false
	}
	// 실제 비디오 payload의 첫 NAL은 보통 ref 정보를 가지므로 nri==0은 배제
	if nri == 0 && (nalType == 1 || nalType == 5 || nalType == 7 || nalType == 8) {
		return false
	}
	return true
}

func estimateFPS(frames []h264Frame) int {
	if len(frames) < 2 {
		return 25
	}
	first, last := frames[0].timestamp, frames[len(frames)-1].timestamp
	if first.IsZero() || last.IsZero() || !last.After(first) {
		return 25
	}
	seconds := last.Sub(first).Seconds()
	if seconds <= 0 {
		return 25
	}
	fps := int(float64(len(frames)-1)/seconds + 0.5)
	if fps < 1 {
		return 1
	}
	if fps > 60 {
		return 60
	}
	return fps
}

func detectVideoDimensions(frames []h264Frame) (int, int) {
	for _, frame := range frames {
		if w, h, ok := parseSPSDimensions(frame.data); ok {
			return w, h
		}
	}
	return 1920, 1080
}

func writeAVI(path string, frames []h264Frame, width, height, fps int) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create avi file: %w", err)
	}
	defer f.Close()

	if _, err := f.Write([]byte("RIFF\x00\x00\x00\x00AVI ")); err != nil {
		return fmt.Errorf("failed to write avi riff header: %w", err)
	}
	if err := writeHDRL(f, width, height, fps, len(frames)); err != nil {
		return err
	}

	moviStart, err := f.Seek(0, 1)
	if err != nil {
		return fmt.Errorf("failed to seek avi movi start: %w", err)
	}
	if _, err := f.Write([]byte("LIST\x00\x00\x00\x00movi")); err != nil {
		return fmt.Errorf("failed to write avi movi header: %w", err)
	}

	type idxEntry struct {
		offset uint32
		size   uint32
		flags  uint32
	}
	entries := make([]idxEntry, 0, len(frames))
	moviDataStart, _ := f.Seek(0, 1)
	maxFrame := 0

	for _, frame := range frames {
		chunkStart, _ := f.Seek(0, 1)
		offset := uint32(chunkStart - moviDataStart)
		size := uint32(len(frame.data))
		if len(frame.data) > maxFrame {
			maxFrame = len(frame.data)
		}

		if _, err := f.Write([]byte("00dc")); err != nil {
			return fmt.Errorf("failed to write avi chunk id: %w", err)
		}
		if err := binary.Write(f, binary.LittleEndian, size); err != nil {
			return fmt.Errorf("failed to write avi chunk size: %w", err)
		}
		if _, err := f.Write(frame.data); err != nil {
			return fmt.Errorf("failed to write avi frame data: %w", err)
		}
		if size%2 == 1 {
			if _, err := f.Write([]byte{0x00}); err != nil {
				return fmt.Errorf("failed to write avi chunk padding: %w", err)
			}
		}

		flags := uint32(0)
		if frame.keyFrame {
			flags = 0x10
		}
		entries = append(entries, idxEntry{offset: offset, size: size, flags: flags})
	}

	moviEnd, _ := f.Seek(0, 1)
	moviSize := uint32(moviEnd - moviStart - 8)
	if _, err := f.Seek(moviStart+4, 0); err != nil {
		return fmt.Errorf("failed to patch movi size: %w", err)
	}
	if err := binary.Write(f, binary.LittleEndian, moviSize); err != nil {
		return fmt.Errorf("failed to write movi size: %w", err)
	}
	if _, err := f.Seek(moviEnd, 0); err != nil {
		return fmt.Errorf("failed to seek movi end: %w", err)
	}

	if _, err := f.Write([]byte("idx1")); err != nil {
		return fmt.Errorf("failed to write idx1 id: %w", err)
	}
	if err := binary.Write(f, binary.LittleEndian, uint32(len(entries)*16)); err != nil {
		return fmt.Errorf("failed to write idx1 size: %w", err)
	}
	for _, entry := range entries {
		if _, err := f.Write([]byte("00dc")); err != nil {
			return fmt.Errorf("failed to write idx1 ckid: %w", err)
		}
		if err := binary.Write(f, binary.LittleEndian, entry.flags); err != nil {
			return fmt.Errorf("failed to write idx1 flags: %w", err)
		}
		if err := binary.Write(f, binary.LittleEndian, entry.offset); err != nil {
			return fmt.Errorf("failed to write idx1 offset: %w", err)
		}
		if err := binary.Write(f, binary.LittleEndian, entry.size); err != nil {
			return fmt.Errorf("failed to write idx1 size field: %w", err)
		}
	}

	fileEnd, _ := f.Seek(0, 1)
	if _, err := f.Seek(4, 0); err != nil {
		return fmt.Errorf("failed to seek riff size patch: %w", err)
	}
	if err := binary.Write(f, binary.LittleEndian, uint32(fileEnd-8)); err != nil {
		return fmt.Errorf("failed to write riff size: %w", err)
	}

	if maxFrame == 0 {
		return fmt.Errorf("empty frame payload")
	}
	return nil
}

func writeHDRL(f *os.File, width, height, fps, totalFrames int) error {
	microSecPerFrame := uint32(1_000_000 / max(1, fps))
	suggestedBuffer := uint32(width * height)
	if suggestedBuffer < 1024*1024 {
		suggestedBuffer = 1024 * 1024
	}

	hdrlSize := uint32(4 + (8 + 56) + (8 + 4 + (8 + 64) + (8 + 40)))
	if _, err := f.Write([]byte("LIST")); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, hdrlSize); err != nil {
		return err
	}
	if _, err := f.Write([]byte("hdrl")); err != nil {
		return err
	}

	if _, err := f.Write([]byte("avih")); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint32(56)); err != nil {
		return err
	}
	avih := []uint32{
		microSecPerFrame,                     // dwMicroSecPerFrame
		uint32(width * height * max(1, fps)), // dwMaxBytesPerSec
		0,                                    // dwPaddingGranularity
		0x10,                                 // dwFlags (AVIF_HASINDEX)
		uint32(totalFrames),                  // dwTotalFrames
		0,                                    // dwInitialFrames
		1,                                    // dwStreams
		suggestedBuffer,                      // dwSuggestedBufferSize
		uint32(width),                        // dwWidth
		uint32(height),                       // dwHeight
		0, 0, 0, 0,                           // dwReserved[4]
	}
	for _, v := range avih {
		if err := binary.Write(f, binary.LittleEndian, v); err != nil {
			return err
		}
	}

	strlSize := uint32(4 + (8 + 64) + (8 + 40))
	if _, err := f.Write([]byte("LIST")); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, strlSize); err != nil {
		return err
	}
	if _, err := f.Write([]byte("strl")); err != nil {
		return err
	}

	if _, err := f.Write([]byte("strh")); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint32(64)); err != nil {
		return err
	}
	if _, err := f.Write([]byte("vidsH264")); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint32(0)); err != nil { // flags
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint16(0)); err != nil { // priority
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint16(0)); err != nil { // language
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint32(0)); err != nil { // initial frames
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint32(1)); err != nil { // scale
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint32(max(1, fps))); err != nil { // rate
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint32(0)); err != nil { // start
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint32(totalFrames)); err != nil { // length
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, suggestedBuffer); err != nil { // suggested buffer
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint32(0xFFFFFFFF)); err != nil { // quality
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint32(0)); err != nil { // sample size
		return err
	}
	for _, v := range []int16{0, 0, int16(width), int16(height)} {
		if err := binary.Write(f, binary.LittleEndian, v); err != nil {
			return err
		}
	}

	if _, err := f.Write([]byte("strf")); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint32(40)); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint32(40)); err != nil { // biSize
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, int32(width)); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, int32(height)); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint16(1)); err != nil { // planes
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint16(24)); err != nil { // bitcount
		return err
	}
	if _, err := f.Write([]byte("H264")); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint32(0)); err != nil { // image size
		return err
	}
	for range 4 {
		if err := binary.Write(f, binary.LittleEndian, int32(0)); err != nil { // ppm and colors
			return err
		}
	}

	return nil
}

func parseSPSDimensions(frame []byte) (int, int, bool) {
	nalus := splitAnnexBNALUs(frame)
	for _, nalu := range nalus {
		if len(nalu) < 2 {
			continue
		}
		nalType := nalu[0] & 0x1F
		if nalType != 7 {
			continue
		}
		w, h, ok := decodeSPS(nalu[1:])
		if ok {
			return w, h, true
		}
	}
	return 0, 0, false
}

func splitAnnexBNALUs(data []byte) [][]byte {
	out := make([][]byte, 0)
	start := -1
	i := 0
	for i+3 < len(data) {
		scLen := 0
		if data[i] == 0 && data[i+1] == 0 && data[i+2] == 1 {
			scLen = 3
		} else if i+4 < len(data) && data[i] == 0 && data[i+1] == 0 && data[i+2] == 0 && data[i+3] == 1 {
			scLen = 4
		}
		if scLen > 0 {
			if start >= 0 && i > start {
				out = append(out, data[start:i])
			}
			start = i + scLen
			i += scLen
			continue
		}
		i++
	}
	if start >= 0 && start < len(data) {
		out = append(out, data[start:])
	}
	return out
}

func decodeSPS(payload []byte) (int, int, bool) {
	rbsp := removeEmulationPrevention(payload)
	br := &bitReader{data: rbsp}

	profileIDC, ok := br.readBits(8)
	if !ok {
		return 0, 0, false
	}
	if _, ok = br.readBits(8); !ok { // constraints
		return 0, 0, false
	}
	if _, ok = br.readBits(8); !ok { // level idc
		return 0, 0, false
	}
	if _, ok = br.readUE(); !ok { // seq_parameter_set_id
		return 0, 0, false
	}

	if profileNeedsExtended(profileIDC) {
		chromaFormatIDC, ok := br.readUE()
		if !ok {
			return 0, 0, false
		}
		if chromaFormatIDC == 3 {
			if _, ok = br.readBits(1); !ok { // separate_colour_plane_flag
				return 0, 0, false
			}
		}
		if _, ok = br.readUE(); !ok { // bit_depth_luma_minus8
			return 0, 0, false
		}
		if _, ok = br.readUE(); !ok { // bit_depth_chroma_minus8
			return 0, 0, false
		}
		if _, ok = br.readBits(1); !ok { // qpprime_y_zero_transform_bypass_flag
			return 0, 0, false
		}
		scaling, ok := br.readBits(1)
		if !ok {
			return 0, 0, false
		}
		if scaling == 1 {
			return 0, 0, false
		}
	}

	if _, ok = br.readUE(); !ok { // log2_max_frame_num_minus4
		return 0, 0, false
	}
	pocType, ok := br.readUE()
	if !ok {
		return 0, 0, false
	}
	if pocType == 0 {
		if _, ok = br.readUE(); !ok {
			return 0, 0, false
		}
	} else if pocType == 1 {
		if _, ok = br.readBits(1); !ok {
			return 0, 0, false
		}
		if _, ok = br.readSE(); !ok {
			return 0, 0, false
		}
		if _, ok = br.readSE(); !ok {
			return 0, 0, false
		}
		numRef, ok := br.readUE()
		if !ok {
			return 0, 0, false
		}
		for range numRef {
			if _, ok = br.readSE(); !ok {
				return 0, 0, false
			}
		}
	}

	if _, ok = br.readUE(); !ok { // max_num_ref_frames
		return 0, 0, false
	}
	if _, ok = br.readBits(1); !ok { // gaps_in_frame_num_value_allowed_flag
		return 0, 0, false
	}
	picWidthInMbsMinus1, ok := br.readUE()
	if !ok {
		return 0, 0, false
	}
	picHeightInMapUnitsMinus1, ok := br.readUE()
	if !ok {
		return 0, 0, false
	}
	frameMbsOnlyFlag, ok := br.readBits(1)
	if !ok {
		return 0, 0, false
	}
	if frameMbsOnlyFlag == 0 {
		if _, ok = br.readBits(1); !ok {
			return 0, 0, false
		}
	}
	if _, ok = br.readBits(1); !ok { // direct_8x8_inference_flag
		return 0, 0, false
	}
	frameCroppingFlag, ok := br.readBits(1)
	if !ok {
		return 0, 0, false
	}

	var cropLeft, cropRight, cropTop, cropBottom uint
	if frameCroppingFlag == 1 {
		if cropLeft, ok = br.readUE(); !ok {
			return 0, 0, false
		}
		if cropRight, ok = br.readUE(); !ok {
			return 0, 0, false
		}
		if cropTop, ok = br.readUE(); !ok {
			return 0, 0, false
		}
		if cropBottom, ok = br.readUE(); !ok {
			return 0, 0, false
		}
	}

	width := int((picWidthInMbsMinus1 + 1) * 16)
	heightMul := 2
	if frameMbsOnlyFlag == 1 {
		heightMul = 1
	}
	height := int((picHeightInMapUnitsMinus1 + 1) * uint(heightMul) * 16)

	width -= int((cropLeft + cropRight) * 2)
	height -= int((cropTop + cropBottom) * 2)
	if width <= 0 || height <= 0 {
		return 0, 0, false
	}
	return width, height, true
}

func removeEmulationPrevention(src []byte) []byte {
	out := make([]byte, 0, len(src))
	for i := 0; i < len(src); i++ {
		if i+2 < len(src) && src[i] == 0x00 && src[i+1] == 0x00 && src[i+2] == 0x03 {
			out = append(out, 0x00, 0x00)
			i += 2
			continue
		}
		out = append(out, src[i])
	}
	return out
}

func profileNeedsExtended(profile uint) bool {
	switch profile {
	case 100, 110, 122, 244, 44, 83, 86, 118, 128, 138, 139, 134, 135:
		return true
	default:
		return false
	}
}

type bitReader struct {
	data []byte
	pos  uint
}

func (b *bitReader) readBits(n uint) (uint, bool) {
	var out uint
	for range n {
		bytePos := b.pos / 8
		if int(bytePos) >= len(b.data) {
			return 0, false
		}
		bitPos := 7 - (b.pos % 8)
		bit := (b.data[bytePos] >> bitPos) & 0x01
		out = (out << 1) | uint(bit)
		b.pos++
	}
	return out, true
}

func (b *bitReader) readUE() (uint, bool) {
	var zeros uint
	for {
		bit, ok := b.readBits(1)
		if !ok {
			return 0, false
		}
		if bit == 1 {
			break
		}
		zeros++
	}
	if zeros == 0 {
		return 0, true
	}
	val, ok := b.readBits(zeros)
	if !ok {
		return 0, false
	}
	return (1<<zeros - 1) + val, true
}

func (b *bitReader) readSE() (int, bool) {
	ue, ok := b.readUE()
	if !ok {
		return 0, false
	}
	if ue%2 == 0 {
		return -int(ue / 2), true
	}
	return int((ue + 1) / 2), true
}
