package image_stream

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAsyncImageTerminalProviderFailureSwitchesChannel(t *testing.T) {
	failed, backup, task := setupCheckpointedProviderFailoverTest(t, "terminal-provider-failure")

	genericImageExecutorRegistry.Lock()
	previousExecutor := genericImageExecutorRegistry.executor
	genericImageExecutorRegistry.executor = func(_ context.Context, request *GenericImageExecutionRequest) (*GenericImageExecutionResult, *types.NewAPIError) {
		require.NoError(t, request.BeforeProviderCall())
		require.NoError(t, request.Checkpoint(&GenericImageUpstreamResponse{
			StatusCode: http.StatusAccepted,
			Body:       json.RawMessage(`{"task_id":"upstream-task-failed"}`),
		}))
		return nil, types.NewErrorWithStatusCode(
			errors.New("provider task reached terminal failed state"),
			types.ErrorCodeBadResponseBody,
			http.StatusBadGateway,
			types.ErrOptionWithSkipRetry(),
		)
	}
	genericImageExecutorRegistry.Unlock()
	t.Cleanup(func() {
		genericImageExecutorRegistry.Lock()
		genericImageExecutorRegistry.executor = previousExecutor
		genericImageExecutorRegistry.Unlock()
	})

	completed, executeErr := executeAsyncImageTask(context.Background(), task)

	assertAsyncImageFailoverScheduled(t, failed, backup, task, completed, executeErr)
}

func TestAsyncImageCheckpointedProviderNotFoundSwitchesChannel(t *testing.T) {
	failed, backup, task := setupCheckpointedProviderFailoverTest(t, "provider-task-not-found")

	installAsyncImageFailoverExecutor(t, func(request *GenericImageExecutionRequest) (*GenericImageExecutionResult, *types.NewAPIError) {
		require.NoError(t, request.BeforeProviderCall())
		require.NoError(t, request.Checkpoint(&GenericImageUpstreamResponse{
			StatusCode: http.StatusAccepted,
			Body:       json.RawMessage(`{"task_id":"upstream-task-lost"}`),
		}))
		return nil, types.NewErrorWithStatusCode(
			errors.New("provider no longer recognizes the accepted task"),
			types.ErrorCodeBadResponseStatusCode,
			http.StatusNotFound,
			types.ErrOptionWithSkipRetry(),
		)
	})

	completed, executeErr := executeAsyncImageTask(context.Background(), task)

	assertAsyncImageFailoverScheduled(t, failed, backup, task, completed, executeErr)
}

func TestAsyncImageEmptyAcceptedProviderResultSwitchesChannel(t *testing.T) {
	failed, backup, task := setupCheckpointedProviderFailoverTest(t, "empty-provider-result")

	installAsyncImageFailoverExecutor(t, func(request *GenericImageExecutionRequest) (*GenericImageExecutionResult, *types.NewAPIError) {
		require.NoError(t, request.BeforeProviderCall())
		require.NoError(t, request.Checkpoint(&GenericImageUpstreamResponse{
			StatusCode: http.StatusAccepted,
			Body:       json.RawMessage(`{"task_id":"upstream-task-empty"}`),
		}))
		return &GenericImageExecutionResult{Response: &dto.ImageResponse{}}, nil
	})

	completed, executeErr := executeAsyncImageTask(context.Background(), task)

	assertAsyncImageFailoverScheduled(t, failed, backup, task, completed, executeErr)
}

func TestAsyncImageAcceptedCountMismatchSwitchesChannel(t *testing.T) {
	failed, backup, task := setupCheckpointedProviderFailoverTest(t, "output-contract")
	payload := decodeStoredAsyncImagePayload(t, task.CheckpointData)
	payload.ImageRequirement.N = 1
	task.SetCheckpointData(payload)

	encoded := base64.StdEncoding.EncodeToString(asyncOutputContractPNG(t, 1, 1))
	installAsyncImageFailoverExecutor(t, func(request *GenericImageExecutionRequest) (*GenericImageExecutionResult, *types.NewAPIError) {
		require.NoError(t, request.BeforeProviderCall())
		response := &dto.ImageResponse{Data: []dto.ImageData{{B64Json: encoded}, {B64Json: encoded}}}
		body, err := common.Marshal(response)
		require.NoError(t, err)
		require.NoError(t, request.Checkpoint(&GenericImageUpstreamResponse{StatusCode: http.StatusAccepted, Body: body}))
		return &GenericImageExecutionResult{Response: response}, nil
	})

	completed, executeErr := executeAsyncImageTask(context.Background(), task)

	assertAsyncImageFailoverScheduled(t, failed, backup, task, completed, executeErr)
}

func TestAsyncImageAcceptedPermanentMaterializationFailureSwitchesChannel(t *testing.T) {
	failed, backup, task := setupCheckpointedProviderFailoverTest(t, "permanent-materialization")
	fetchSetting := system_setting.GetFetchSetting()
	previousFetchSetting := *fetchSetting
	fetchSetting.EnableSSRFProtection = false
	t.Cleanup(func() { *fetchSetting = previousFetchSetting })

	providerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, err := w.Write([]byte("not an image"))
		require.NoError(t, err)
	}))
	t.Cleanup(providerServer.Close)
	previousImageSourceClient := getGenericImageSourceClient
	getGenericImageSourceClient = func() genericImageHTTPClient { return providerServer.Client() }
	t.Cleanup(func() { getGenericImageSourceClient = previousImageSourceClient })

	installAsyncImageFailoverExecutor(t, func(request *GenericImageExecutionRequest) (*GenericImageExecutionResult, *types.NewAPIError) {
		require.NoError(t, request.BeforeProviderCall())
		providerURL := providerServer.URL + "/invalid"
		require.NoError(t, request.Checkpoint(&GenericImageUpstreamResponse{
			StatusCode: http.StatusAccepted,
			Body:       json.RawMessage(fmt.Sprintf(`{"task_id":"upstream-invalid-output","result":{"data":[{"url":%q}]}}`, providerURL)),
		}))
		return &GenericImageExecutionResult{
			Response: &dto.ImageResponse{Data: []dto.ImageData{{Url: providerURL}}},
		}, nil
	})

	completed, executeErr := executeAsyncImageTask(context.Background(), task)

	assertAsyncImageFailoverScheduled(t, failed, backup, task, completed, executeErr)
}

func TestAsyncImageDefinitiveClientErrorsDoNotSwitchOrCoolChannel(t *testing.T) {
	tests := []struct {
		name      string
		errorCode types.ErrorCode
		message   string
	}{
		{name: "invalid caller parameter", errorCode: types.ErrorCodeInvalidRequest, message: "invalid image size"},
		{name: "content policy rejection", errorCode: types.ErrorCodePromptBlocked, message: "prompt was rejected by content policy"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			failed, _, task := setupCheckpointedProviderFailoverTest(t, "client-error-"+test.name)
			installAsyncImageFailoverExecutor(t, func(request *GenericImageExecutionRequest) (*GenericImageExecutionResult, *types.NewAPIError) {
				require.NoError(t, request.BeforeProviderCall())
				return nil, types.NewErrorWithStatusCode(
					fmt.Errorf("%w: %s", ErrGenericImageDefinitiveResponse, test.message),
					test.errorCode,
					http.StatusBadRequest,
					types.ErrOptionWithSkipRetry(),
				)
			})

			completed, executeErr := executeAsyncImageTask(context.Background(), task)

			assert.False(t, completed)
			assert.NoError(t, executeErr, "caller and policy errors are terminal for the logical request")
			var stored model.Task
			require.NoError(t, model.DB.First(&stored, task.ID).Error)
			assert.Equal(t, failed.Id, stored.ChannelId)
			assert.Equal(t, model.TaskStatus(model.TaskStatusFailure), stored.Status)
			assert.Zero(t, stored.ProviderAttempts)
			assert.False(t, model.IsChannelCoolingDown(failed.Id))
		})
	}
}

func TestCompatibleAsyncImageFailoverAllowsCrossTypeLosslessImageRoute(t *testing.T) {
	failed := &model.Channel{Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Models: "gpt-image-2"}
	candidate := &model.Channel{Type: constant.ChannelTypeSiliconFlow, Status: common.ChannelStatusEnabled, Models: "gpt-image-2"}
	failed.SetOtherSettings(dto.ChannelOtherSettings{ImageRouting: asyncImageFailoverTestRouting()})
	candidate.SetOtherSettings(dto.ChannelOtherSettings{ImageRouting: asyncImageFailoverTestRouting()})
	payload := asyncImageTaskPayload{
		Executor:                 AsyncImageExecutorAdaptor,
		RelayMode:                relayconstant.RelayModeImagesGenerations,
		ImageRoutingProtocol:     dto.ImageRoutingProtocolImagesGenerations,
		ImageRoutingUpstreamPath: "/v1/images/generations",
		ImageRequirement: &dto.ImageSelectionRequirement{
			Operation: dto.ImageOperationGeneration,
			Size:      "auto",
			N:         1,
		},
		Request: &dto.ImageRequest{Model: "gpt-image-2", Prompt: "lossless failover", Size: "auto", ResponseFormat: "url"},
	}
	task := &model.Task{Properties: model.Properties{OriginModelName: "gpt-image-2", UpstreamModelName: "gpt-image-2"}}

	assert.True(t, compatibleAsyncImageFailoverChannel(failed, candidate, task, &payload),
		"channel type alone must not block failover when the frozen image requirement and target profile are lossless")
}

func TestAsyncImageAdaptorTimeoutIsProtocolAware(t *testing.T) {
	assert.Equal(t, 35*time.Second, asyncImageAdaptorTimeout(asyncImageTaskPayload{
		ImageRoutingProtocol: dto.ImageRoutingProtocolKIEJobs,
	}))
	assert.Equal(t, asyncImageUpstreamTimeout, asyncImageAdaptorTimeout(asyncImageTaskPayload{
		ImageRoutingProtocol: dto.ImageRoutingProtocolImagesGenerations,
	}))
}

func TestAsyncImageKIEPendingPollKeepsDeadlineWithoutConsumingFailureBudget(t *testing.T) {
	_, _, task := setupCheckpointedProviderFailoverTest(t, "kie-pending-deadline")
	payload := decodeStoredAsyncImagePayload(t, task.CheckpointData)
	payload.ImageRoutingProtocol = dto.ImageRoutingProtocolKIEJobs
	payload.ImageRoutingUpstreamPath = "/api/v1/jobs/createTask"
	payload.PreparedRequest.ImageRoutingProtocol = dto.ImageRoutingProtocolKIEJobs
	payload.PreparedRequest.ImageRoutingUpstreamPath = "/api/v1/jobs/createTask"
	task.SetCheckpointData(payload)
	require.NoError(t, model.DB.Model(&model.Task{}).Where("id = ?", task.ID).Update("checkpoint_data", task.CheckpointData).Error)

	installAsyncImageFailoverExecutor(t, func(request *GenericImageExecutionRequest) (*GenericImageExecutionResult, *types.NewAPIError) {
		if request.UpstreamResponse == nil {
			require.NoError(t, request.BeforeProviderCall())
			require.NoError(t, request.Checkpoint(&GenericImageUpstreamResponse{
				StatusCode: http.StatusAccepted,
				Body:       json.RawMessage(`{"code":200,"data":{"taskId":"kie-pending-task"}}`),
			}))
		}
		return nil, types.NewErrorWithStatusCode(
			types.NewProviderTaskPollingRetryError(errors.New("provider task is pending"), 7*time.Second),
			types.ErrorCodeBadResponse,
			http.StatusAccepted,
		)
	})

	before := time.Now()
	completed, executeErr := executeAsyncImageTask(context.Background(), task)
	require.False(t, completed)
	require.ErrorIs(t, executeErr, errAsyncImageRetryScheduled)
	var stored model.Task
	require.NoError(t, model.DB.First(&stored, task.ID).Error)
	assert.Zero(t, stored.ProviderAttempts)
	firstPayload := decodeStoredAsyncImagePayload(t, stored.CheckpointData)
	require.NotZero(t, firstPayload.ProviderDeadlineAt)
	deadline := time.Unix(firstPayload.ProviderDeadlineAt, 0)
	assert.GreaterOrEqual(t, deadline, before.Add(14*time.Minute))
	assert.LessOrEqual(t, deadline, before.Add(16*time.Minute))

	for poll := 0; poll < 8; poll++ {
		stored = model.Task{}
		require.NoError(t, model.DB.First(&stored, task.ID).Error)
		require.Equal(t, model.TaskStatus(model.TaskStatusNotStart), stored.Status)
		claimed, err := model.ClaimImageTask(&stored, common.GetTimestamp())
		require.NoError(t, err)
		require.True(t, claimed)
		completed, executeErr = executeAsyncImageTask(context.Background(), &stored)
		require.False(t, completed)
		require.ErrorIs(t, executeErr, errAsyncImageRetryScheduled)
		stored = model.Task{}
		require.NoError(t, model.DB.First(&stored, task.ID).Error)
		assert.Zero(t, stored.ProviderAttempts, "normal pending polls must not consume the provider failure budget")
		currentPayload := decodeStoredAsyncImagePayload(t, stored.CheckpointData)
		assert.Equal(t, firstPayload.ProviderDeadlineAt, currentPayload.ProviderDeadlineAt, "worker restarts must not reset the overall provider deadline")
	}
}

func TestAsyncImageKIEProviderDeadlineSwitchesChannel(t *testing.T) {
	failed, backup, task := setupCheckpointedProviderFailoverTest(t, "kie-expired-deadline")
	payload := decodeStoredAsyncImagePayload(t, task.CheckpointData)
	payload.ImageRoutingProtocol = dto.ImageRoutingProtocolKIEJobs
	payload.ImageRoutingUpstreamPath = "/api/v1/jobs/createTask"
	payload.PreparedRequest.ImageRoutingProtocol = dto.ImageRoutingProtocolKIEJobs
	payload.PreparedRequest.ImageRoutingUpstreamPath = "/api/v1/jobs/createTask"
	payload.ProviderStored = true
	payload.ProviderDeadlineAt = time.Now().Add(-time.Second).Unix()
	checkpoint, err := common.Marshal(payload)
	require.NoError(t, err)
	artifact, err := common.Marshal(GenericImageUpstreamResponse{
		StatusCode: http.StatusAccepted,
		Body:       json.RawMessage(`{"code":200,"data":{"taskId":"kie-expired-task"}}`),
	})
	require.NoError(t, err)
	persisted, err := model.PersistImageTaskArtifact(task, checkpoint, artifact, "40%")
	require.NoError(t, err)
	require.True(t, persisted)

	var pollCalls atomic.Int32
	installAsyncImageFailoverExecutor(t, func(*GenericImageExecutionRequest) (*GenericImageExecutionResult, *types.NewAPIError) {
		pollCalls.Add(1)
		return nil, types.NewErrorWithStatusCode(
			types.NewProviderTaskPollingRetryError(errors.New("provider task is pending"), time.Second),
			types.ErrorCodeBadResponse,
			http.StatusAccepted,
		)
	})

	completed, executeErr := executeAsyncImageTask(context.Background(), task)

	assertAsyncImageFailoverScheduled(t, failed, backup, task, completed, executeErr)
	assert.Zero(t, pollCalls.Load())
	reset := decodeStoredAsyncImagePayload(t, task.CheckpointData)
	assert.Zero(t, reset.ProviderDeadlineAt)
}

func TestAsyncImageFailoverCheckpointStoresOnlyDestinationFingerprints(t *testing.T) {
	_, backup, task := setupCheckpointedProviderFailoverTest(t, "checkpoint-secrets")
	credentialedBaseURL := "https://base-user:base-secret@backup-checkpoint.example.com"
	backup.BaseURL = &credentialedBaseURL
	backup.SetSetting(dto.ChannelSettings{Proxy: "http://proxy-user:proxy-secret@proxy.example.com:8080"})
	paramOverride := `{"provider_token":"param-secret"}`
	headerOverride := `{"X-Provider-Secret":"header-secret"}`
	backup.ParamOverride = &paramOverride
	backup.HeaderOverride = &headerOverride

	payload := decodeStoredAsyncImagePayload(t, task.CheckpointData)
	started, err := beginAsyncImageProviderCall(task, &payload)
	require.NoError(t, err)
	require.True(t, started)

	switched, err := switchRejectedAsyncImageChannel(context.Background(), task, &payload, "upstream returned status 503")
	require.NoError(t, err)
	require.True(t, switched)

	var stored model.Task
	require.NoError(t, model.DB.First(&stored, task.ID).Error)
	plaintext, err := model.DecryptImageTaskArtifactCheckpoint(stored.CheckpointData)
	require.NoError(t, err)
	checkpoint := decodeStoredAsyncImagePayload(t, stored.CheckpointData)
	assert.Empty(t, checkpoint.ChannelBaseURL)
	assert.Empty(t, checkpoint.ChannelProxy)
	require.NotNil(t, checkpoint.PreparedRequest)
	assert.Empty(t, checkpoint.PreparedRequest.ChannelBaseURL)
	require.NotNil(t, checkpoint.PreparedRequest.ChannelSetting)
	assert.Empty(t, checkpoint.PreparedRequest.ChannelSetting.Proxy)
	for _, secret := range []string{"base-secret", "proxy-secret", "param-secret", "header-secret"} {
		assert.NotContains(t, string(plaintext), secret)
	}
	assert.True(t, checkpoint.ExecutionDestinationStored)
	assert.NotEmpty(t, checkpoint.ExecutionDestinationHash)
	assert.True(t, checkpoint.PreparedRequest.ExecutionOverrideStored)
	assert.NotEmpty(t, checkpoint.PreparedRequest.ExecutionOverrideHash)
}

func TestAsyncImageFailoverAppliesCandidateProfileDefaults(t *testing.T) {
	_, backup, task := setupCheckpointedProviderFailoverTest(t, "candidate-defaults")
	backup.SetOtherSettings(dto.ChannelOtherSettings{ImageRouting: &dto.ImageRoutingConfig{
		Version: dto.ImageRoutingVersion1,
		Profiles: []dto.ImageRoutingProfile{{
			Model:              "gpt-image-2",
			Protocol:           dto.ImageRoutingProtocolKIEJobs,
			UpstreamPath:       "/api/v1/jobs/createTask",
			Operations:         []dto.ImageOperation{dto.ImageOperationGeneration},
			Resolutions:        []string{"2K"},
			AspectRatios:       []string{"16:9"},
			Sizes:              []string{"auto"},
			DefaultResolution:  "2K",
			DefaultAspectRatio: "16:9",
			DefaultSize:        "auto",
			MaxOutputImages:    1,
			AllowedCombinations: []dto.ImageRoutingCombination{{
				Operation: dto.ImageOperationGeneration, Resolution: "2K", AspectRatio: "16:9", Size: "auto",
			}},
			VerificationStatus: dto.ImageRoutingVerificationProductionVerified,
		}},
	}})

	payload := decodeStoredAsyncImagePayload(t, task.CheckpointData)
	require.Empty(t, payload.ImageRequirement.Resolution)
	require.Empty(t, payload.ImageRequirement.AspectRatio)
	started, err := beginAsyncImageProviderCall(task, &payload)
	require.NoError(t, err)
	require.True(t, started)

	switched, err := switchRejectedAsyncImageChannel(context.Background(), task, &payload, "upstream returned status 503")
	require.NoError(t, err)
	require.True(t, switched)

	require.NotNil(t, payload.ImageRequirement)
	assert.Equal(t, "2K", payload.ImageRequirement.Resolution)
	assert.Equal(t, "16:9", payload.ImageRequirement.AspectRatio)
	assert.Equal(t, dto.ImageRoutingProtocolKIEJobs, payload.ImageRoutingProtocol)
	assert.JSONEq(t, `"2K"`, string(payload.RequestExtra["resolution"]))
	assert.JSONEq(t, `"16:9"`, string(payload.RequestExtra["aspect_ratio"]))
	require.NotNil(t, payload.Request)
	require.NoError(t, payload.Request.SetImageSelectionRequirement(*payload.ImageRequirement))
	requirement, ok := payload.Request.ImageSelectionRequirement()
	require.True(t, ok)
	assert.Equal(t, "2K", requirement.Resolution)
	assert.Equal(t, "16:9", requirement.AspectRatio)
}

func installAsyncImageFailoverExecutor(t *testing.T, execute func(*GenericImageExecutionRequest) (*GenericImageExecutionResult, *types.NewAPIError)) {
	t.Helper()
	genericImageExecutorRegistry.Lock()
	previousExecutor := genericImageExecutorRegistry.executor
	genericImageExecutorRegistry.executor = func(_ context.Context, request *GenericImageExecutionRequest) (*GenericImageExecutionResult, *types.NewAPIError) {
		return execute(request)
	}
	genericImageExecutorRegistry.Unlock()
	t.Cleanup(func() {
		genericImageExecutorRegistry.Lock()
		genericImageExecutorRegistry.executor = previousExecutor
		genericImageExecutorRegistry.Unlock()
	})
}

func assertAsyncImageFailoverScheduled(t *testing.T, failed, backup *model.Channel, task *model.Task, completed bool, executeErr error) {
	t.Helper()
	assert.False(t, completed)
	assert.ErrorIs(t, executeErr, errAsyncImageRetryScheduled)
	var stored model.Task
	require.NoError(t, model.DB.First(&stored, task.ID).Error)
	assert.Equal(t, model.TaskStatus(model.TaskStatusNotStart), stored.Status)
	assert.Equal(t, backup.Id, stored.ChannelId)
	assert.Equal(t, 1, stored.ProviderAttempts)
	assert.NotEqual(t, failed.Id, stored.ChannelId)
}

func setupCheckpointedProviderFailoverTest(t *testing.T, suffix string) (*model.Channel, *model.Channel, *model.Task) {
	t.Helper()
	setupAsyncImageSubmitTestDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Channel{}, &model.ImageTaskArtifactChunk{}, &model.Log{}))

	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = true
	model.ClearChannelCacheForTest()
	model.ClearChannelCooldownsForTest()
	t.Cleanup(func() {
		model.ClearChannelCacheForTest()
		model.ClearChannelCooldownsForTest()
		common.MemoryCacheEnabled = previousMemoryCacheEnabled
	})
	previousLogDB := model.LOG_DB
	model.LOG_DB = model.DB
	t.Cleanup(func() { model.LOG_DB = previousLogDB })

	priority := int64(10)
	weight := uint(100)
	failedBaseURL := "https://failed-" + suffix + ".example.com"
	backupBaseURL := "https://backup-" + suffix + ".example.com"
	failed := &model.Channel{
		Id: 9116, Type: constant.ChannelTypeOpenAI, Key: "failed-key", Status: common.ChannelStatusEnabled,
		Name: "failed " + suffix, CreatedTime: 1700001100, BaseURL: &failedBaseURL,
		Models: "gpt-image-2", Group: "default", Priority: &priority, Weight: &weight,
	}
	backup := &model.Channel{
		Id: 9118, Type: constant.ChannelTypeOpenAI, Key: "backup-key", Status: common.ChannelStatusEnabled,
		Name: "backup " + suffix, CreatedTime: 1700001200, BaseURL: &backupBaseURL,
		Models: "gpt-image-2", Group: "default", Priority: &priority, Weight: &weight,
	}
	failed.SetOtherSettings(dto.ChannelOtherSettings{ImageRouting: asyncImageFailoverTestRouting()})
	backup.SetOtherSettings(dto.ChannelOtherSettings{ImageRouting: asyncImageFailoverTestRouting()})
	require.NoError(t, model.DB.Create(failed).Error)
	require.NoError(t, model.DB.Create(backup).Error)
	model.SetChannelCacheForTest(map[int]*model.Channel{failed.Id: failed, backup.Id: backup}, map[string]map[string][]int{
		"default": {"gpt-image-2": {failed.Id, backup.Id}},
	})

	user := &model.User{
		Username: "async-failover-" + suffix,
		Status:   common.UserStatusEnabled,
		Group:    "default",
	}
	require.NoError(t, model.DB.Create(user).Error)
	payload := asyncImageTaskPayload{
		Version:                  asyncImagePayloadVersion,
		Executor:                 AsyncImageExecutorAdaptor,
		RelayMode:                relayconstant.RelayModeImagesGenerations,
		ImageRoutingProtocol:     dto.ImageRoutingProtocolImagesGenerations,
		ImageRoutingUpstreamPath: "/v1/images/generations",
		ImageRequirement: &dto.ImageSelectionRequirement{
			Operation: dto.ImageOperationGeneration,
			Size:      "auto",
			N:         1,
		},
		Request:             &dto.ImageRequest{Model: "gpt-image-2", Prompt: "draw a reliable image", Size: "auto", ResponseFormat: "url"},
		ChannelType:         failed.Type,
		ChannelCreateTime:   failed.CreatedTime,
		AttemptedChannelIDs: []int{failed.Id},
		PreparedRequest: &PreparedAsyncImageRequest{
			Body:                     []byte(`{"model":"gpt-image-2","prompt":"draw a reliable image","size":"auto","response_format":"url"}`),
			RelayMode:                relayconstant.RelayModeImagesGenerations,
			ContentType:              "application/json",
			RequestURLPath:           "/v1/images/generations",
			ImageRoutingProtocol:     dto.ImageRoutingProtocolImagesGenerations,
			ImageRoutingUpstreamPath: "/v1/images/generations",
			APIType:                  constant.APITypeOpenAI,
			ChannelType:              failed.Type,
			ChannelCreateTime:        failed.CreatedTime,
		},
	}
	task := &model.Task{
		TaskID: "task_" + suffix, Platform: constant.TaskPlatformOpenAIImage,
		UserId: user.Id, Group: "default", ChannelId: failed.Id, Status: model.TaskStatusInProgress,
		Attempt: 1, Progress: "10%",
		Properties: model.Properties{OriginModelName: "gpt-image-2", UpstreamModelName: "gpt-image-2"},
		PrivateData: model.TaskPrivateData{
			ChannelKeyHash: common.GenerateHMAC(failed.Key),
			BillingContext: &model.TaskBillingContext{PerCallBilling: true, OriginModelName: "gpt-image-2"},
		},
	}
	task.SetCheckpointData(payload)
	require.NoError(t, model.DB.Create(task).Error)
	return failed, backup, task
}

func asyncImageFailoverTestRouting() *dto.ImageRoutingConfig {
	return &dto.ImageRoutingConfig{
		Version: dto.ImageRoutingVersion1,
		Profiles: []dto.ImageRoutingProfile{{
			Model:           "gpt-image-2",
			Protocol:        dto.ImageRoutingProtocolImagesGenerations,
			UpstreamPath:    "/v1/images/generations",
			Operations:      []dto.ImageOperation{dto.ImageOperationGeneration},
			Sizes:           []string{"auto", "2x1"},
			DefaultSize:     "auto",
			MaxOutputImages: 1,
			AllowedCombinations: []dto.ImageRoutingCombination{
				{Operation: dto.ImageOperationGeneration, Size: "auto"},
				{Operation: dto.ImageOperationGeneration, Size: "2x1"},
			},
			VerificationStatus: dto.ImageRoutingVerificationProductionVerified,
		}},
	}
}

func TestAsyncImageTransportFailureSwitchesChannelAndPreservesCause(t *testing.T) {
	channel, backup, task := setupCheckpointedProviderFailoverTest(t, "transport-timeout-cause")
	calls := 0
	installAsyncImageFailoverExecutor(t, func(request *GenericImageExecutionRequest) (*GenericImageExecutionResult, *types.NewAPIError) {
		require.NoError(t, request.BeforeProviderCall())
		calls++
		return nil, types.NewErrorWithStatusCode(errors.New("image request exceeded the gateway request timeout"), types.ErrorCodeDoRequestFailed, http.StatusGatewayTimeout)
	})
	completed, err := executeAsyncImageTask(context.Background(), task)
	assertAsyncImageFailoverScheduled(t, channel, backup, task, completed, err)
	var stored model.Task
	require.NoError(t, model.DB.First(&stored, task.ID).Error)
	assert.Contains(t, stored.ProviderError, "gateway request timeout")
	assert.NotContains(t, stored.ProviderError, "interrupted checkpoint")
	assert.Equal(t, 1, calls)
}

func TestAsyncImageFailoverNeverReturnsToAlreadyFailedChannel(t *testing.T) {
	failed, backup, task := setupCheckpointedProviderFailoverTest(t, "failed-channel-exclusion")
	// Legacy payloads may omit their initial channel from attempted_channel_ids.
	payload := decodeStoredAsyncImagePayload(t, task.CheckpointData)
	payload.AttemptedChannelIDs = nil
	task.SetCheckpointData(payload)
	calls := []int{}
	installAsyncImageFailoverExecutor(t, func(request *GenericImageExecutionRequest) (*GenericImageExecutionResult, *types.NewAPIError) {
		require.NoError(t, request.BeforeProviderCall())
		calls = append(calls, request.RelayInfo.ChannelId)
		return nil, types.NewErrorWithStatusCode(errors.New("provider image timed out"), types.ErrorCodeDoRequestFailed, http.StatusGatewayTimeout)
	})
	completed, err := executeAsyncImageTask(context.Background(), task)
	assertAsyncImageFailoverScheduled(t, failed, backup, task, completed, err)
	rotated := decodeStoredAsyncImagePayload(t, task.CheckpointData)
	assert.Equal(t, []int{failed.Id, backup.Id}, rotated.AttemptedChannelIDs)
	assert.False(t, rotated.ProviderStored)
	assert.False(t, rotated.ProviderCallStarted)
	claimed, err := model.ClaimImageTask(task, common.GetTimestamp())
	require.NoError(t, err)
	require.True(t, claimed)
	completed, err = executeAsyncImageTask(context.Background(), task)
	assert.False(t, completed)
	require.NoError(t, err)
	assert.Equal(t, []int{failed.Id, backup.Id}, calls)
	var stored model.Task
	require.NoError(t, model.DB.First(&stored, task.ID).Error)
	assert.Equal(t, model.TaskStatus(model.TaskStatusFailure), stored.Status)
	assert.Equal(t, backup.Id, stored.ChannelId)
	assert.Contains(t, stored.FailReason, "provider image timed out")
}

func TestAsyncImageFailoverSettlesOrRefundsOnce(t *testing.T) {
	for _, succeeds := range []bool{true, false} {
		t.Run(fmt.Sprint(succeeds), func(t *testing.T) {
			failed, backup, task := setupCheckpointedProviderFailoverTest(t, "settlement-"+fmt.Sprint(succeeds))
			task.Quota = 100
			task.PrivateData.BillingSource = "wallet"
			require.NoError(t, model.DB.Save(task).Error)
			require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", task.UserId).Update("quota", 900).Error)
			require.NoError(t, model.DB.Create(&model.ImageBillingReservation{
				TaskID: task.TaskID, UserID: task.UserId, ExpectedQuota: 100,
				FundingSource: "wallet", WalletReserved: 100, Status: model.ImageBillingReservationActive,
			}).Error)
			previousClient := http.DefaultClient
			http.DefaultClient = &http.Client{Transport: asyncImageRoundTripFunc(func(request *http.Request) (*http.Response, error) {
				assert.Equal(t, http.MethodPut, request.Method)
				assert.Equal(t, "test-account.r2.cloudflarestorage.com", request.URL.Host)
				return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("")), Request: request}, nil
			})}
			t.Cleanup(func() { http.DefaultClient = previousClient })
			encoded := base64.StdEncoding.EncodeToString(asyncOutputContractPNG(t, 1, 1))
			calls := []int{}
			installAsyncImageFailoverExecutor(t, func(request *GenericImageExecutionRequest) (*GenericImageExecutionResult, *types.NewAPIError) {
				assert.Nil(t, request.UpstreamResponse, "a new channel cannot receive the previous provider's checkpoint")
				require.NoError(t, request.BeforeProviderCall())
				calls = append(calls, request.RelayInfo.ChannelId)
				response := &dto.ImageResponse{Data: []dto.ImageData{{B64Json: encoded}}}
				body, err := common.Marshal(response)
				require.NoError(t, err)
				require.NoError(t, request.Checkpoint(&GenericImageUpstreamResponse{StatusCode: http.StatusOK, Body: body}))
				if request.RelayInfo.ChannelId == failed.Id || !succeeds {
					return nil, types.NewErrorWithStatusCode(errors.New("provider task failed"), types.ErrorCodeBadResponseBody, http.StatusBadGateway)
				}
				return &GenericImageExecutionResult{Response: response, Usage: &dto.Usage{PromptTokens: 1, TotalTokens: 1}}, nil
			})
			completed, err := executeAsyncImageTask(context.Background(), task)
			assertAsyncImageFailoverScheduled(t, failed, backup, task, completed, err)
			var user model.User
			require.NoError(t, model.DB.First(&user, task.UserId).Error)
			assert.Equal(t, 900, user.Quota, "switching retains the original reservation")
			assert.Zero(t, user.UsedQuota)
			claimed, err := model.ClaimImageTask(task, common.GetTimestamp())
			require.NoError(t, err)
			require.True(t, claimed)
			completed, err = executeAsyncImageTask(context.Background(), task)
			require.NoError(t, err)
			assert.Equal(t, succeeds, completed)
			assert.Equal(t, []int{failed.Id, backup.Id}, calls)
			finalization, err := model.FinalizeImageTask(task.TaskID)
			require.NoError(t, err)
			assert.False(t, finalization.Applied, "repeated finalization must not settle or refund again")
			require.NoError(t, model.DB.First(&user, task.UserId).Error)
			if succeeds {
				assert.Equal(t, 900, user.Quota)
				assert.Equal(t, 100, user.UsedQuota)
				assert.Equal(t, model.TaskStatus(model.TaskStatusSuccess), finalization.Task.Status)
				assert.NotEmpty(t, finalization.Task.PrivateData.ResultURL)
			} else {
				assert.Equal(t, 1000, user.Quota)
				assert.Zero(t, user.UsedQuota)
				assert.Equal(t, model.TaskStatus(model.TaskStatusFailure), finalization.Task.Status)
			}
		})
	}
}

func TestResponsesImageProviderFailuresSwitchChannel(t *testing.T) {
	for _, failure := range []string{"http503", "disconnect", "invalid-image"} {
		t.Run(failure, func(t *testing.T) {
			failed, backup, task := setupCheckpointedProviderFailoverTest(t, "responses-"+failure)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				assert.Equal(t, "/v1/responses", request.URL.Path)
				_, _ = io.Copy(io.Discard, request.Body)
				switch failure {
				case "http503":
					w.WriteHeader(http.StatusServiceUnavailable)
				case "disconnect":
					conn, _, err := w.(http.Hijacker).Hijack()
					if assert.NoError(t, err) {
						_ = conn.Close()
					}
				case "invalid-image":
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = io.WriteString(w, "data: {\"type\":\"response.output_item.done\",\"item\":{\"type\":\"image_generation_call\",\"result\":\"bm90LWFuLWltYWdl\"}}\n\n")
					_, _ = io.WriteString(w, "data: {\"type\":\"response.completed\",\"response\":{\"model\":\"gpt-image-2\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n")
				}
			}))
			t.Cleanup(upstream.Close)
			failed.BaseURL = &upstream.URL
			require.NoError(t, model.DB.Save(failed).Error)
			payload := decodeStoredAsyncImagePayload(t, task.CheckpointData)
			payload.Executor = AsyncImageExecutorResponses
			payload.PreparedRequest = nil
			payload.ImageRoutingProtocol = dto.ImageRoutingProtocolResponsesSSE
			payload.ImageRoutingUpstreamPath = "/v1/responses"
			task.SetCheckpointData(payload)
			completed, err := executeAsyncImageTask(context.Background(), task)
			assertAsyncImageFailoverScheduled(t, failed, backup, task, completed, err)
		})
	}
}

func TestAsyncImageQuickFailureRetriesOnceThenSwitchesWithoutCooling(t *testing.T) {
	failed, backup, task := setupCheckpointedProviderFailoverTest(t, "quick-retry")
	calls := 0
	installAsyncImageFailoverExecutor(t, func(request *GenericImageExecutionRequest) (*GenericImageExecutionResult, *types.NewAPIError) {
		calls++
		assert.Equal(t, failed.Id, task.ChannelId)
		assert.Equal(t, task.TaskID, request.RelayInfo.RequestId)
		require.NoError(t, request.BeforeProviderCall())
		if calls == 2 {
			var stored model.Task
			require.NoError(t, model.DB.First(&stored, task.ID).Error)
			persisted := decodeStoredAsyncImagePayload(t, stored.CheckpointData)
			assert.True(t, persisted.SameChannelRetryUsed)
			assert.True(t, persisted.ProviderCallStarted)
			assert.Equal(t, model.TaskStatus(model.TaskStatusCheckpointPending), stored.Status)
		}
		apiErr := types.NewErrorWithStatusCode(fmt.Errorf("%w: temporary gateway failure", ErrGenericImageDefinitiveResponse), types.ErrorCodeBadResponseStatusCode, http.StatusBadGateway)
		apiErr.UpstreamStatusCode = http.StatusBadGateway
		return nil, apiErr
	})
	completed, err := executeAsyncImageTask(context.Background(), task)
	assert.Equal(t, 2, calls)
	assertAsyncImageFailoverScheduled(t, failed, backup, task, completed, err)
	assert.False(t, model.IsChannelCoolingDown(failed.Id))
	assert.False(t, decodeStoredAsyncImagePayload(t, task.CheckpointData).SameChannelRetryUsed, "a new channel gets its own single retry")
}

func TestAsyncImageSameChannelRetryBoundaries(t *testing.T) {
	tests := []struct {
		name                      string
		elapsed                   time.Duration
		status                    int
		used, accepted, ambiguous bool
		want                      bool
	}{
		{name: "within twenty seconds", elapsed: 20 * time.Second, status: 502, want: true},
		{name: "slow rejection rotates immediately", elapsed: 20*time.Second + time.Nanosecond, status: 502},
		{name: "retry already persisted", elapsed: time.Second, status: 502, used: true},
		{name: "accepted work", elapsed: time.Second, status: 502, accepted: true},
		{name: "transport failure", elapsed: time.Second, status: 502, ambiguous: true},
		{name: "rate limited", elapsed: time.Second, status: 429},
		{name: "invalid request", elapsed: time.Second, status: 400},
		{name: "invalid credentials", elapsed: time.Second, status: 401},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, task := setupCheckpointedProviderFailoverTest(t, "retry-boundary")
			payload := decodeStoredAsyncImagePayload(t, task.CheckpointData)
			started, err := beginAsyncImageProviderCall(task, &payload)
			require.NoError(t, err)
			require.True(t, started)
			payload.SameChannelRetryUsed = tt.used
			payload.ProviderStored = tt.accepted
			cause := fmt.Errorf("%w: temporary failure", ErrGenericImageDefinitiveResponse)
			if tt.ambiguous {
				cause = errors.New("connection reset")
			}
			apiErr := types.NewErrorWithStatusCode(cause, types.ErrorCodeBadResponseStatusCode, tt.status)
			apiErr.UpstreamStatusCode = tt.status
			retried, err := retryRejectedAsyncImageCall(task, &payload, apiErr, tt.elapsed)
			require.NoError(t, err)
			assert.Equal(t, tt.want, retried)
			if tt.want {
				var stored model.Task
				require.NoError(t, model.DB.First(&stored, task.ID).Error)
				persisted := decodeStoredAsyncImagePayload(t, stored.CheckpointData)
				assert.True(t, persisted.SameChannelRetryUsed)
				assert.False(t, persisted.ProviderCallStarted)
				assert.Equal(t, model.TaskStatus(model.TaskStatusInProgress), stored.Status)
				assert.Zero(t, stored.ProviderAttempts)
				assert.Equal(t, task.Quota, stored.Quota)
			}
		})
	}
}

func TestAsyncImageQuickRetryCanDeliverWithoutSwitching(t *testing.T) {
	failed, _, task := setupCheckpointedProviderFailoverTest(t, "retry-delivered")
	previousTransport := http.DefaultClient.Transport
	uploads := 0
	http.DefaultClient.Transport = asyncImageRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		assert.Equal(t, http.MethodPut, request.Method)
		uploads++
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
	})
	t.Cleanup(func() { http.DefaultClient.Transport = previousTransport })
	calls := 0
	encoded := base64.StdEncoding.EncodeToString(asyncOutputContractPNG(t, 1, 1))
	installAsyncImageFailoverExecutor(t, func(request *GenericImageExecutionRequest) (*GenericImageExecutionResult, *types.NewAPIError) {
		calls++
		require.NoError(t, request.BeforeProviderCall())
		if calls == 1 {
			apiErr := types.NewErrorWithStatusCode(fmt.Errorf("%w: temporary gateway failure", ErrGenericImageDefinitiveResponse), types.ErrorCodeBadResponseStatusCode, http.StatusBadGateway)
			apiErr.UpstreamStatusCode = http.StatusBadGateway
			return nil, apiErr
		}
		response := &dto.ImageResponse{Data: []dto.ImageData{{B64Json: encoded}}}
		body, err := common.Marshal(response)
		require.NoError(t, err)
		require.NoError(t, request.Checkpoint(&GenericImageUpstreamResponse{StatusCode: http.StatusOK, Body: body}))
		return &GenericImageExecutionResult{Response: response}, nil
	})
	completed, err := executeAsyncImageTask(context.Background(), task)
	require.NoError(t, err)
	assert.True(t, completed)
	assert.Equal(t, 2, calls)
	assert.Equal(t, 1, uploads)
	var stored model.Task
	require.NoError(t, model.DB.First(&stored, task.ID).Error)
	assert.Equal(t, model.TaskStatus(model.TaskStatusSuccess), stored.Status)
	assert.Equal(t, failed.Id, stored.ChannelId)
	assert.Zero(t, stored.ProviderAttempts)
	assert.False(t, model.IsChannelCoolingDown(failed.Id))
}
