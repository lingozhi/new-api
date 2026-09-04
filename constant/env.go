package constant

var StreamingTimeout int
var DifyDebug bool
var MaxFileDownloadMB int
var StreamScannerMaxBufferMB int
var ForceStreamOption bool
var CountToken bool
var GetMediaToken bool
var GetMediaTokenNotStream bool
var UpdateTask bool
var MaxRequestBodyMB int
var AnonymousRequestBodyLimitKB int

// UploadIdleTimeoutSeconds bounds how long a request body may deliver nothing
// before the request is abandoned. 0 disables it (wait forever, the old
// behaviour).
var UploadIdleTimeoutSeconds int
var AzureDefaultAPIVersion string
var NotifyLimitCount int
var NotificationLimitDurationMinute int
var GenerateDefaultToken bool
var ErrorLogEnabled bool
var TaskQueryLimit int
var TaskTimeoutMinutes int

// temporary variable for sora patch, will be removed in future
var TaskPricePatches []string

// TaskPricePerSecondModels use a fixed per-second price but reconcile the
// maximum submit-time precharge to the provider-reported duration on completion.
var TaskPricePerSecondModels = []string{
	ArgolinkSeedance25Model,
	ArgolinkSeedance20Model,
	"MiniMax-H3",
	"depth-anything-v2-small-video",
	"subtitle-remove",
	"video-upscale-quality-2x",
	"video-upscale-quality-4x",
	"video-background-remove-fast",
	"video-background-remove-quality",
}

// TrustedRedirectDomains is a list of trusted domains for redirect URL validation.
// Domains support subdomain matching (e.g., "example.com" matches "sub.example.com").
var TrustedRedirectDomains []string
