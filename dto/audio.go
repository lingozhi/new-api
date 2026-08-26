package dto

import (
	"encoding/json"
	"strings"

	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

type AudioRequest struct {
	Model          string          `json:"model"`
	Input          string          `json:"input"`
	Voice          string          `json:"voice"`
	Instructions   string          `json:"instructions,omitempty"`
	ResponseFormat string          `json:"response_format,omitempty"`
	Speed          *float64        `json:"speed,omitempty"`
	StreamFormat   string          `json:"stream_format,omitempty"`
	Metadata       json.RawMessage `json:"metadata,omitempty"`
	// vllm-omini
	TaskType                json.RawMessage `json:"task_type,omitempty"`
	Language                json.RawMessage `json:"language,omitempty"`
	RefAudio                json.RawMessage `json:"ref_audio,omitempty"`
	RefText                 json.RawMessage `json:"ref_text,omitempty"`
	XVectorOnlyMode         json.RawMessage `json:"x_vector_only_mode,omitempty"`
	MaxNewTokens            json.RawMessage `json:"max_new_tokens,omitempty"`
	InitialCodecChunkFrames json.RawMessage `json:"initial_codec_chunk_frames,omitempty"`
	// TODO：ensure that the logic remains correct after the stream is started.
	//Stream                  json.RawMessage `json:"stream,omitempty"`
}

// IndexTTS2Metadata carries the IndexTTS2 emotion controls exposed through
// the otherwise OpenAI-compatible audio speech request. The vector order is
// happy, angry, sad, afraid, disgusted, melancholic, surprised, calm.
type IndexTTS2Metadata struct {
	EmotionAudio  *string    `json:"emotion_audio,omitempty"`
	EmotionVector *[]float64 `json:"emotion_vector,omitempty"`
	EmotionRandom *bool      `json:"emotion_random,omitempty"`
}

// IndexTTS2SpeechRequest accepts both the model's complete workflow contract
// and the earlier OpenAI Speech aliases. The workflow fields are pointers so
// explicit zero and false values survive request decoding.
type IndexTTS2SpeechRequest struct {
	AudioRequest
	PromptText       *string         `json:"prompt_text,omitempty"`
	PromptSimple     *string         `json:"prompt_simple,omitempty"`
	EmoControlMethod *string         `json:"emo_control_method,omitempty"`
	EmoAfraid        *float64        `json:"emo_afraid,omitempty"`
	EmoAngry         *float64        `json:"emo_angry,omitempty"`
	EmoCalm          *float64        `json:"emo_calm,omitempty"`
	EmoDisgusted     *float64        `json:"emo_disgusted,omitempty"`
	EmoHappy         *float64        `json:"emo_happy,omitempty"`
	EmoMelancholic   *float64        `json:"emo_melancholic,omitempty"`
	EmoRandom        *bool           `json:"emo_random,omitempty"`
	EmoRefAudio      *string         `json:"emo_ref_audio,omitempty"`
	EmoSad           *float64        `json:"emo_sad,omitempty"`
	EmoSurprised     json.RawMessage `json:"emo_surprised,omitempty"`
}

func (r *IndexTTS2SpeechRequest) EffectivePromptText() string {
	if r != nil && r.PromptText != nil {
		return *r.PromptText
	}
	if r == nil {
		return ""
	}
	return r.Input
}

func (r *IndexTTS2SpeechRequest) EffectivePromptSimple() string {
	if r != nil && r.PromptSimple != nil {
		return *r.PromptSimple
	}
	if r == nil {
		return ""
	}
	return r.Voice
}

func (r *AudioRequest) GetTokenCountMeta() *types.TokenCountMeta {
	meta := &types.TokenCountMeta{
		CombineText: r.Input,
		TokenType:   types.TokenTypeTextNumber,
	}
	if strings.Contains(r.Model, "gpt") {
		meta.TokenType = types.TokenTypeTokenizer
	}
	return meta
}

func (r *AudioRequest) IsStream(c *gin.Context) bool {
	return r.StreamFormat == "sse"
}

func (r *AudioRequest) SetModelName(modelName string) {
	if modelName != "" {
		r.Model = modelName
	}
}

type AudioResponse struct {
	Text string `json:"text"`
}

type WhisperVerboseJSONResponse struct {
	Task     string    `json:"task,omitempty"`
	Language string    `json:"language,omitempty"`
	Duration float64   `json:"duration,omitempty"`
	Text     string    `json:"text,omitempty"`
	Segments []Segment `json:"segments,omitempty"`
}

type Segment struct {
	Id               int     `json:"id"`
	Seek             int     `json:"seek"`
	Start            float64 `json:"start"`
	End              float64 `json:"end"`
	Text             string  `json:"text"`
	Tokens           []int   `json:"tokens"`
	Temperature      float64 `json:"temperature"`
	AvgLogprob       float64 `json:"avg_logprob"`
	CompressionRatio float64 `json:"compression_ratio"`
	NoSpeechProb     float64 `json:"no_speech_prob"`
}
