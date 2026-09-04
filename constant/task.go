package constant

type TaskPlatform string

const (
	TaskPlatformSuno        TaskPlatform = "suno"
	TaskPlatformMidjourney               = "mj"
	TaskPlatformOpenAIImage              = "openai_image"
	TaskPlatformAutoDL                   = "60"
)

const ArgolinkSeedance20Model = "seedance-2.0"

const ArgolinkSeedance25Model = "seedance-2.5"

const (
	SunoActionMusic  = "MUSIC"
	SunoActionLyrics = "LYRICS"

	TaskActionGenerate          = "generate"
	TaskActionTextGenerate      = "textGenerate"
	TaskActionFirstTailGenerate = "firstTailGenerate"
	TaskActionReferenceGenerate = "referenceGenerate"
	TaskActionRemix             = "remixGenerate"
	TaskActionVideoGenerationV2 = "videoGenerationV2"
	TaskActionAudioSpeech       = "audioSpeech"
)

var SunoModel2Action = map[string]string{
	"suno_music":  SunoActionMusic,
	"suno_lyrics": SunoActionLyrics,
}
