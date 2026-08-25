package autodl

type workflowResponse struct {
	Code      string               `json:"code"`
	Data      workflowResponseData `json:"data"`
	Msg       string               `json:"msg"`
	RequestID string               `json:"request_id"`
}

type workflowResponseData struct {
	TaskID    string           `json:"task_id"`
	Status    string           `json:"status"`
	Message   string           `json:"message"`
	Error     any              `json:"error"`
	CreatedAt string           `json:"created_at"`
	Results   []workflowResult `json:"results"`
}

type workflowResult struct {
	URL        string `json:"url"`
	Type       string `json:"type"`
	FileType   string `json:"file_type"`
	OutputType string `json:"output_type"`
}

type contentSummary struct {
	Prompt          string
	FirstFrame      string
	LastFrame       string
	ReferenceImages []string
	ReferenceAudios []string
}
