package autodl

import "github.com/QuantumNous/new-api/constant"

const ChannelName = "autodl"

var ModelList = []string{constant.AutoDLModelMiniMaxH3, constant.AutoDLModelIndexTTS2}

const (
	workflowTextToVideo            = "minimax_h3_lightx2v_no_pic"
	workflowFirstLastFrame         = "minimax_h3_lightx2v"
	workflowReferenceImages        = "minimax_h3_lightx2v_v5"
	workflowReferenceImages15s     = "minimax_h3_lightx2v_v5_15s"
	workflowImageAudioSync         = "minimax_h3_image_audio_to_video"
	workflowReferenceImageAudio    = "minimax_h3_image_audio_to_video_v2"
	workflowReferenceImageAudio15s = "minimax_h3_image_audio_to_video_v2_15s"
	workflowIndexTTS2              = constant.AutoDLModelIndexTTS2
)

const (
	indexTTSControlSameAsVoice   = "与音色参考音频相同"
	indexTTSControlEmotionAudio  = "使用情感参考音频"
	indexTTSControlEmotionVector = "使用情感向量控制"
)

const (
	autoDLStatusQueued    = "QUEUED"
	autoDLStatusRunning   = "RUNNING"
	autoDLStatusSuccess   = "SUCCESS"
	autoDLStatusCompleted = "COMPLETED"
	autoDLStatusFailed    = "FAILED"
	autoDLStatusFailure   = "FAILURE"
)

const (
	miniMaxRoleFirstFrame     = "first_frame"
	miniMaxRoleLastFrame      = "last_frame"
	miniMaxRoleReferenceImage = "reference_image"
	miniMaxRoleReferenceVideo = "reference_video"
	miniMaxRoleReferenceAudio = "reference_audio"
)
