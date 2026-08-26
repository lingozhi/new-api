package constant

const (
	AutoDLModelMiniMaxH3 = "MiniMax-H3"
	AutoDLModelIndexTTS2 = "indextts2-v1"
)

const (
	AutoDLVideoGenerationV2Path = "/v2/video_generation"
	AutoDLAudioSpeechPath       = "/v1/audio/speech"
)

// AutoDLSupportsRequest keeps AutoDL's workflow-specific models from being
// selected for an endpoint their adapters cannot implement.
func AutoDLSupportsRequest(requestPath, modelName string) bool {
	switch requestPath {
	case AutoDLVideoGenerationV2Path:
		return modelName == AutoDLModelMiniMaxH3
	case AutoDLAudioSpeechPath:
		return modelName == AutoDLModelIndexTTS2
	default:
		return false
	}
}
