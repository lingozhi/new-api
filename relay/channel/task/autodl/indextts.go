package autodl

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
)

var indexTTSEmotionFields = [...]string{
	"emo_happy",
	"emo_angry",
	"emo_sad",
	"emo_afraid",
	"emo_disgusted",
	"emo_melancholic",
	"emo_surprised",
	"emo_calm",
}

func buildIndexTTSWorkflowRequest(request *dto.AudioRequest) (string, map[string]any, error) {
	if request == nil {
		return "", nil, errors.New("request is required")
	}
	if request.Model != constant.AutoDLModelIndexTTS2 {
		return "", nil, fmt.Errorf("model must be %s", constant.AutoDLModelIndexTTS2)
	}
	if !utf8.ValidString(request.Input) {
		return "", nil, errors.New("input must be valid UTF-8")
	}
	inputLength := utf8.RuneCountInString(request.Input)
	if inputLength < 1 || inputLength > 2048 || strings.TrimSpace(request.Input) == "" {
		return "", nil, errors.New("input must contain between 1 and 2048 characters")
	}
	if strings.TrimSpace(request.Instructions) != "" {
		return "", nil, errors.New("instructions are not supported by indextts2-v1")
	}
	responseFormat := strings.ToLower(strings.TrimSpace(request.ResponseFormat))
	if responseFormat != "" && responseFormat != "wav" {
		return "", nil, errors.New("response_format must be wav")
	}
	if request.Speed != nil && *request.Speed != 1 {
		return "", nil, errors.New("speed must be 1 for indextts2-v1")
	}
	if strings.EqualFold(strings.TrimSpace(request.StreamFormat), "sse") {
		return "", nil, errors.New("SSE streaming is not supported by indextts2-v1")
	}

	voice, totalDataBytes, err := validateMediaURL(request.Voice, mediaKindAudio, 0)
	if err != nil {
		return "", nil, fmt.Errorf("voice is invalid: %w", err)
	}
	payload := map[string]any{
		"prompt_text":        request.Input,
		"prompt_simple":      voice,
		"emo_control_method": indexTTSControlSameAsVoice,
		"emo_random":         false,
	}

	var metadata dto.IndexTTS2Metadata
	if len(request.Metadata) > 0 {
		if err := common.Unmarshal(request.Metadata, &metadata); err != nil {
			return "", nil, errors.New("metadata must be a valid IndexTTS2 metadata object")
		}
	}

	hasEmotionAudio := metadata.EmotionAudio != nil
	hasEmotionVector := metadata.EmotionVector != nil
	hasRandomEmotion := metadata.EmotionRandom != nil && *metadata.EmotionRandom
	activeEmotionControls := 0
	for _, active := range []bool{hasEmotionAudio, hasEmotionVector, hasRandomEmotion} {
		if active {
			activeEmotionControls++
		}
	}
	if activeEmotionControls > 1 {
		return "", nil, errors.New("emotion_audio, emotion_vector, and emotion_random=true are mutually exclusive")
	}

	if hasEmotionAudio {
		emotionAudio, _, err := validateMediaURL(*metadata.EmotionAudio, mediaKindAudio, totalDataBytes)
		if err != nil {
			return "", nil, fmt.Errorf("emotion_audio is invalid: %w", err)
		}
		payload["emo_control_method"] = indexTTSControlEmotionAudio
		payload["emo_ref_audio"] = emotionAudio
	}

	if hasEmotionVector {
		vector := *metadata.EmotionVector
		if len(vector) != len(indexTTSEmotionFields) {
			return "", nil, fmt.Errorf("emotion_vector must contain exactly %d values", len(indexTTSEmotionFields))
		}
		for index, value := range vector {
			if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 1.4 {
				return "", nil, errors.New("emotion_vector values are outside the allowed range of 0 to 1.4")
			}
			if index == 6 && value != 0 {
				return "", nil, errors.New("emotion_vector surprised value must be 0")
			}
			payload[indexTTSEmotionFields[index]] = value
		}
		payload["emo_control_method"] = indexTTSControlEmotionVector
	}

	if metadata.EmotionRandom != nil {
		payload["emo_random"] = *metadata.EmotionRandom
		if *metadata.EmotionRandom {
			payload["emo_control_method"] = indexTTSControlEmotionVector
		}
	}

	return workflowIndexTTS2, payload, nil
}
