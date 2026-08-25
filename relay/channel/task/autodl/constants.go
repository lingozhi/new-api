package autodl

const ChannelName = "autodl"

var ModelList = []string{"MiniMax-H3"}

const (
	workflowTextToVideo            = "minimax_h3_lightx2v_no_pic"
	workflowReferenceImages        = "minimax_h3_lightx2v_v5"
	workflowReferenceImages15s     = "minimax_h3_lightx2v_v5_15s"
	workflowReferenceImageAudio    = "minimax_h3_image_audio_to_video_v2"
	workflowReferenceImageAudio15s = "minimax_h3_image_audio_to_video_v2_15s"
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
