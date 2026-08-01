package kie

const (
	ChannelName = "kie"

	ModelGPTImage2TextToImage  = "gpt-image-2-text-to-image"
	ModelGPTImage2ImageToImage = "gpt-image-2-image-to-image"

	createTaskPath = "/api/v1/jobs/createTask"
	taskRecordPath = "/api/v1/jobs/recordInfo"
)

var ModelList = []string{"gpt-image-2"}
