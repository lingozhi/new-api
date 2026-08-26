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

func buildIndexTTSWorkflowRequest(request *dto.IndexTTS2SpeechRequest) (string, map[string]any, error) {
	if request == nil {
		return "", nil, errors.New("request is required")
	}
	if request.Model != constant.AutoDLModelIndexTTS2 {
		return "", nil, fmt.Errorf("model must be %s", constant.AutoDLModelIndexTTS2)
	}
	if request.PromptText != nil && request.Input != "" && request.Input != *request.PromptText {
		return "", nil, errors.New("input and prompt_text must match when both are provided")
	}
	promptText := request.EffectivePromptText()
	promptTextField := "input"
	if request.PromptText != nil {
		promptTextField = "prompt_text"
	}
	if !utf8.ValidString(promptText) {
		return "", nil, fmt.Errorf("%s must be valid UTF-8", promptTextField)
	}
	inputLength := utf8.RuneCountInString(promptText)
	if inputLength < 1 || inputLength > 2048 || strings.TrimSpace(promptText) == "" {
		return "", nil, fmt.Errorf("%s must contain between 1 and 2048 characters", promptTextField)
	}
	if request.PromptSimple != nil && request.Voice != "" && request.Voice != *request.PromptSimple {
		return "", nil, errors.New("voice and prompt_simple must match when both are provided")
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

	promptSimpleField := "voice"
	if request.PromptSimple != nil {
		promptSimpleField = "prompt_simple"
	}
	promptSimple, totalDataBytes, err := validateMediaURL(request.EffectivePromptSimple(), mediaKindAudio, 0)
	if err != nil {
		return "", nil, fmt.Errorf("%s is invalid: %w", promptSimpleField, err)
	}
	payload := map[string]any{
		"prompt_text":        promptText,
		"prompt_simple":      promptSimple,
		"emo_control_method": indexTTSControlSameAsVoice,
		"emo_random":         false,
	}

	directEmotionValues := []struct {
		name  string
		value *float64
	}{
		{name: "emo_afraid", value: request.EmoAfraid},
		{name: "emo_angry", value: request.EmoAngry},
		{name: "emo_calm", value: request.EmoCalm},
		{name: "emo_disgusted", value: request.EmoDisgusted},
		{name: "emo_happy", value: request.EmoHappy},
		{name: "emo_melancholic", value: request.EmoMelancholic},
		{name: "emo_sad", value: request.EmoSad},
	}
	hasDirectEmotionVector := len(request.EmoSurprised) > 0
	for _, field := range directEmotionValues {
		if field.value != nil {
			hasDirectEmotionVector = true
			break
		}
	}
	hasDirectEmotionParameters := request.EmoControlMethod != nil || hasDirectEmotionVector ||
		request.EmoRandom != nil || request.EmoRefAudio != nil

	var metadata dto.IndexTTS2Metadata
	if len(request.Metadata) > 0 {
		if hasDirectEmotionParameters {
			return "", nil, errors.New("metadata emotion aliases cannot be combined with emo_* parameters")
		}
		if err := common.Unmarshal(request.Metadata, &metadata); err != nil {
			return "", nil, errors.New("metadata must be a valid IndexTTS2 metadata object")
		}
	}

	if hasDirectEmotionParameters {
		if request.EmoControlMethod != nil {
			controlMethod := strings.TrimSpace(*request.EmoControlMethod)
			switch controlMethod {
			case indexTTSControlSameAsVoice, indexTTSControlEmotionAudio, indexTTSControlEmotionVector:
				payload["emo_control_method"] = controlMethod
			default:
				return "", nil, errors.New("emo_control_method is not supported")
			}
		} else if request.EmoRefAudio != nil {
			payload["emo_control_method"] = indexTTSControlEmotionAudio
		} else if hasDirectEmotionVector || (request.EmoRandom != nil && *request.EmoRandom) {
			payload["emo_control_method"] = indexTTSControlEmotionVector
		}

		for _, field := range directEmotionValues {
			if field.value == nil {
				continue
			}
			value := *field.value
			if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 1.4 {
				return "", nil, fmt.Errorf("%s must be between 0 and 1.4", field.name)
			}
			payload[field.name] = value
		}

		if len(request.EmoSurprised) > 0 {
			var surprised float64
			if err := common.Unmarshal(request.EmoSurprised, &surprised); err != nil {
				var surprisedText string
				if textErr := common.Unmarshal(request.EmoSurprised, &surprisedText); textErr != nil || surprisedText != "0" {
					return "", nil, errors.New("emo_surprised must be 0")
				}
			}
			if surprised != 0 {
				return "", nil, errors.New("emo_surprised must be 0")
			}
			payload["emo_surprised"] = float64(0)
		}

		if request.EmoRandom != nil {
			payload["emo_random"] = *request.EmoRandom
		}
		if request.EmoRefAudio != nil {
			emotionAudio, _, err := validateMediaURL(*request.EmoRefAudio, mediaKindAudio, totalDataBytes)
			if err != nil {
				return "", nil, fmt.Errorf("emo_ref_audio is invalid: %w", err)
			}
			payload["emo_ref_audio"] = emotionAudio
		}

		return workflowIndexTTS2, payload, nil
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
