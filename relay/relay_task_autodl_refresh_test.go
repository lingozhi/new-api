package relay

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/singleflight"
	"gorm.io/gorm"
)

type autoDLResultRefreshTestAdaptor struct {
	channel.TaskAdaptor
	calls   atomic.Int32
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (a *autoDLResultRefreshTestAdaptor) Init(*relaycommon.RelayInfo) {}

func (a *autoDLResultRefreshTestAdaptor) FetchTask(string, string, map[string]any, string) (*http.Response, error) {
	return nil, errors.New("legacy task refresh must not be used")
}

func (a *autoDLResultRefreshTestAdaptor) FetchTaskWithContext(ctx context.Context, _ string, _ string, _ map[string]any, _ string) (*http.Response, error) {
	a.calls.Add(1)
	a.once.Do(func() { close(a.started) })
	select {
	case <-a.release:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewBufferString(`{"code":"Success"}`)),
	}, nil
}

func (a *autoDLResultRefreshTestAdaptor) ParseTaskResult([]byte) (*relaycommon.TaskInfo, error) {
	return &relaycommon.TaskInfo{
		TaskID: "autodl-upstream-refresh",
		Status: model.TaskStatusSuccess,
		Url:    "https://cdn.example.com/refreshed.mp4",
	}, nil
}

func (a *autoDLResultRefreshTestAdaptor) SanitizeTaskResult([]byte) []byte {
	return []byte(`{"code":"Success","data":{"status":"SUCCESS"}}`)
}

func TestRefreshAutoDLSuccessTaskCoalescesConcurrentQueriesAndHonorsTTL(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Task{}))

	previousDB := model.DB
	previousRedisEnabled := common.RedisEnabled
	previousFactory := getTaskAdaptorForResultRefresh
	model.DB = db
	common.RedisEnabled = false
	autoDLResultRefreshGroup = singleflight.Group{}
	t.Cleanup(func() {
		model.DB = previousDB
		common.RedisEnabled = previousRedisEnabled
		getTaskAdaptorForResultRefresh = previousFactory
		autoDLResultRefreshGroup = singleflight.Group{}
		sqlDB, sqlErr := db.DB()
		if sqlErr == nil {
			_ = sqlDB.Close()
		}
	})

	baseURL := "https://autodl.example"
	providerKey := "autodl-refresh-provider-key"
	channelRow := &model.Channel{
		Id:      701,
		Type:    constant.ChannelTypeAutoDL,
		Key:     providerKey,
		Status:  common.ChannelStatusEnabled,
		BaseURL: &baseURL,
	}
	require.NoError(t, db.Create(channelRow).Error)
	now := time.Now().Unix()
	task := &model.Task{
		TaskID:     "task_autodl_refresh_singleflight",
		Platform:   constant.TaskPlatformAutoDL,
		UserId:     77,
		ChannelId:  channelRow.Id,
		Status:     model.TaskStatusSuccess,
		Progress:   "100%",
		SubmitTime: now - 60,
		CreatedAt:  now - 60,
		UpdatedAt:  now - 30,
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID: "autodl-upstream-refresh",
			ResultURL:      "https://cdn.example.com/expired.mp4",
			ChannelKeyHash: common.Sha256([]byte(providerKey)),
		},
	}
	require.NoError(t, db.Create(task).Error)

	fake := &autoDLResultRefreshTestAdaptor{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	getTaskAdaptorForResultRefresh = func(constant.TaskPlatform) channel.TaskAdaptor { return fake }

	const callers = 8
	start := make(chan struct{})
	entered := make(chan struct{}, callers)
	var wg sync.WaitGroup
	wg.Add(callers)
	for range callers {
		go func() {
			defer wg.Done()
			copyOfTask := *task
			<-start
			entered <- struct{}{}
			RefreshAutoDLSuccessTask(context.Background(), &copyOfTask)
		}()
	}
	close(start)
	for range callers {
		<-entered
	}
	<-fake.started
	close(fake.release)
	wg.Wait()

	assert.EqualValues(t, 1, fake.calls.Load())
	var stored model.Task
	require.NoError(t, db.First(&stored, task.ID).Error)
	assert.Equal(t, "https://cdn.example.com/refreshed.mp4", stored.PrivateData.ResultURL)
	assert.Positive(t, stored.PrivateData.ResultRefreshedAt)
	terminalUpdatedAt := stored.UpdatedAt

	RefreshAutoDLSuccessTask(context.Background(), &stored)
	assert.EqualValues(t, 1, fake.calls.Load())
	require.NoError(t, db.First(&stored, task.ID).Error)
	assert.Equal(t, terminalUpdatedAt, stored.UpdatedAt)
}

func TestRefreshAutoDLSuccessTaskSkipsTasksOutsideQueryWindow(t *testing.T) {
	previousFactory := getTaskAdaptorForResultRefresh
	fake := &autoDLResultRefreshTestAdaptor{}
	getTaskAdaptorForResultRefresh = func(constant.TaskPlatform) channel.TaskAdaptor { return fake }
	t.Cleanup(func() { getTaskAdaptorForResultRefresh = previousFactory })

	oldTimestamp := time.Now().Add(-7*24*time.Hour - time.Second).Unix()
	task := &model.Task{
		TaskID:     "task_autodl_expired_result",
		Platform:   constant.TaskPlatformAutoDL,
		Status:     model.TaskStatusSuccess,
		SubmitTime: oldTimestamp,
		CreatedAt:  oldTimestamp,
	}

	RefreshAutoDLSuccessTask(context.Background(), task)

	assert.Zero(t, fake.calls.Load())
}

func TestMiniMaxVideoV2QueryOnlyReturnsAutoDLTasksFromLastSevenDays(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Task{}))
	previousDB := model.DB
	model.DB = db
	t.Cleanup(func() {
		model.DB = previousDB
		sqlDB, sqlErr := db.DB()
		if sqlErr == nil {
			_ = sqlDB.Close()
		}
	})

	now := time.Now().Unix()
	testCases := []struct {
		name        string
		platform    constant.TaskPlatform
		submitTime  int64
		expectError bool
	}{
		{
			name:       "inside seven day window",
			platform:   constant.TaskPlatformAutoDL,
			submitTime: now - int64((7*24*time.Hour)/time.Second) + 2,
		},
		{
			name:        "older than seven days",
			platform:    constant.TaskPlatformAutoDL,
			submitTime:  now - int64((7*24*time.Hour)/time.Second) - 2,
			expectError: true,
		},
		{
			name:        "different task platform",
			platform:    constant.TaskPlatform("kling"),
			submitTime:  now,
			expectError: true,
		},
	}
	for index, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			task := &model.Task{
				TaskID:     "task_minimax_query_window_" + strconv.Itoa(index),
				Platform:   tc.platform,
				UserId:     77,
				Status:     model.TaskStatusQueued,
				SubmitTime: tc.submitTime,
				CreatedAt:  tc.submitTime,
				UpdatedAt:  tc.submitTime,
				Properties: model.Properties{OriginModelName: "MiniMax-H3"},
			}
			require.NoError(t, db.Create(task).Error)

			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodGet, "/v2/query/video_generation/"+task.TaskID, nil)
			c.Params = gin.Params{{Key: "task_id", Value: task.TaskID}}
			c.Set("id", 77)
			body, taskErr := videoFetchByIDRespBodyBuilder(c)
			if tc.expectError {
				require.NotNil(t, taskErr)
				assert.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
				assert.Empty(t, body)
				return
			}
			require.Nil(t, taskErr)
			assert.Contains(t, string(body), task.TaskID)
		})
	}
}
