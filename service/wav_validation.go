package service

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"time"
)

// ValidateWAV checks a seekable RIFF/WAVE stream without materializing it in
// memory. It accepts PCM and IEEE-float audio, requires a coherent fmt chunk
// and non-empty block-aligned audio data, and optionally bounds duration.
func ValidateWAV(reader io.ReadSeeker, size int64, maxDuration time.Duration) error {
	if reader == nil || size < 12 {
		return errors.New("WAV stream is too short")
	}
	if _, err := reader.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("seek WAV header: %w", err)
	}
	header := make([]byte, 12)
	if _, err := io.ReadFull(reader, header); err != nil {
		return errors.New("WAV header is truncated")
	}
	if string(header[:4]) != "RIFF" || string(header[8:12]) != "WAVE" {
		return errors.New("WAV RIFF header is invalid")
	}
	riffEnd := uint64(binary.LittleEndian.Uint32(header[4:8])) + 8
	if riffEnd < 12 || riffEnd != uint64(size) {
		return errors.New("WAV RIFF length does not match the stream")
	}

	var channels, blockAlign, bitsPerSample uint16
	var sampleRate, byteRate uint32
	formatValid := false
	var audioDataBytes uint64
	offset := int64(12)
	chunkHeader := make([]byte, 8)
	for offset+8 <= size {
		if _, err := reader.Seek(offset, io.SeekStart); err != nil {
			return fmt.Errorf("seek WAV chunk: %w", err)
		}
		if _, err := io.ReadFull(reader, chunkHeader); err != nil {
			return errors.New("WAV chunk header is truncated")
		}
		chunkSize := uint64(binary.LittleEndian.Uint32(chunkHeader[4:8]))
		chunkStart := uint64(offset + 8)
		chunkEnd := chunkStart + chunkSize
		if chunkEnd < chunkStart || chunkEnd > riffEnd {
			return errors.New("WAV chunk exceeds the RIFF boundary")
		}

		switch string(chunkHeader[:4]) {
		case "fmt ":
			if formatValid || chunkSize < 16 {
				return errors.New("WAV fmt chunk is invalid")
			}
			formatReadSize := 40
			if chunkSize < uint64(formatReadSize) {
				formatReadSize = int(chunkSize)
			}
			formatBytes := make([]byte, formatReadSize)
			if _, err := reader.Seek(int64(chunkStart), io.SeekStart); err != nil {
				return fmt.Errorf("seek WAV format: %w", err)
			}
			if _, err := io.ReadFull(reader, formatBytes); err != nil {
				return errors.New("WAV fmt chunk is truncated")
			}
			format := binary.LittleEndian.Uint16(formatBytes[0:2])
			channels = binary.LittleEndian.Uint16(formatBytes[2:4])
			sampleRate = binary.LittleEndian.Uint32(formatBytes[4:8])
			byteRate = binary.LittleEndian.Uint32(formatBytes[8:12])
			blockAlign = binary.LittleEndian.Uint16(formatBytes[12:14])
			bitsPerSample = binary.LittleEndian.Uint16(formatBytes[14:16])
			if format == 0xfffe {
				if chunkSize < 40 {
					return errors.New("WAV extensible fmt chunk is truncated")
				}
				format = binary.LittleEndian.Uint16(formatBytes[24:26])
			}
			if (format != 1 && format != 3) || channels == 0 || channels > 8 ||
				sampleRate < 8000 || sampleRate > 192000 || bitsPerSample == 0 || bitsPerSample%8 != 0 {
				return errors.New("WAV audio format is unsupported")
			}
			bytesPerSample := bitsPerSample / 8
			if blockAlign != channels*bytesPerSample || byteRate != sampleRate*uint32(blockAlign) {
				return errors.New("WAV format rates are inconsistent")
			}
			if (format == 1 && bitsPerSample != 8 && bitsPerSample != 16 && bitsPerSample != 24 && bitsPerSample != 32) ||
				(format == 3 && bitsPerSample != 32 && bitsPerSample != 64) {
				return errors.New("WAV sample format is unsupported")
			}
			formatValid = true
		case "data":
			if ^uint64(0)-audioDataBytes < chunkSize {
				return errors.New("WAV audio data length overflowed")
			}
			audioDataBytes += chunkSize
		}

		next := chunkEnd + chunkSize%2
		if next < chunkEnd || next > riffEnd {
			return errors.New("WAV chunk padding exceeds the RIFF boundary")
		}
		offset = int64(next)
	}
	if offset != size || !formatValid || audioDataBytes == 0 || blockAlign == 0 || audioDataBytes%uint64(blockAlign) != 0 {
		return errors.New("WAV stream is missing valid format or audio data")
	}
	if maxDuration > 0 {
		audioDurationNanos := audioDataBytes * uint64(time.Second) / uint64(byteRate)
		if audioDurationNanos > uint64(maxDuration) {
			return errors.New("WAV audio duration exceeds the limit")
		}
	}
	if _, err := reader.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("rewind WAV stream: %w", err)
	}
	return nil
}
