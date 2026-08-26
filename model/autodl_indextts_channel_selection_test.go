package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type autoDLSelectionCase struct {
	name      string
	model     string
	path      string
	wantID    int
	wantNoHit bool
}

var autoDLSelectionMatrix = []autoDLSelectionCase{
	{name: "MiniMax video uses AutoDL", model: "MiniMax-H3", path: "/v2/video_generation", wantID: 701},
	{name: "MiniMax speech cannot use AutoDL", model: "MiniMax-H3", path: "/v1/audio/speech", wantID: 702},
	{name: "IndexTTS speech uses AutoDL", model: "indextts2-v1", path: "/v1/audio/speech", wantID: 701},
	{name: "IndexTTS video has no compatible channel", model: "indextts2-v1", path: "/v2/video_generation", wantNoHit: true},
	{name: "IndexTTS chat cannot use AutoDL", model: "indextts2-v1", path: "/v1/chat/completions", wantID: 702},
	{name: "ordinary OpenAI speech remains available", model: "tts-1", path: "/v1/audio/speech", wantID: 702},
	{name: "unrelated video model cannot use AutoDL", model: "tts-1", path: "/v2/video_generation", wantNoHit: true},
}

func TestAutoDLRequestPathAndModelMatrixWithoutMemoryCache(t *testing.T) {
	setupChannelSelectionTestDB(t)

	priority := int64(10)
	weight := uint(100)
	channels := []Channel{
		{
			Id: 701, Type: constant.ChannelTypeAutoDL, Key: "autodl-key",
			Status: common.ChannelStatusEnabled, Weight: &weight, Priority: &priority,
			Models: "MiniMax-H3,indextts2-v1,tts-1", Group: "default",
		},
		{
			Id: 702, Type: constant.ChannelTypeOpenAI, Key: "openai-key",
			Status: common.ChannelStatusEnabled, Weight: &weight, Priority: &priority,
			Models: "MiniMax-H3,indextts2-v1,tts-1", Group: "default",
		},
	}
	require.NoError(t, DB.Create(&channels).Error)

	abilities := make([]Ability, 0, 6)
	for _, modelName := range []string{"MiniMax-H3", "indextts2-v1", "tts-1"} {
		abilities = append(abilities,
			Ability{Group: "default", Model: modelName, ChannelId: 701, Enabled: true, Priority: &priority, Weight: weight},
			Ability{Group: "default", Model: modelName, ChannelId: 702, Enabled: true, Priority: &priority, Weight: weight},
		)
	}
	require.NoError(t, DB.Create(&abilities).Error)

	for _, test := range autoDLSelectionMatrix {
		t.Run(test.name, func(t *testing.T) {
			selected, err := GetChannelWithOptions("default", test.model, 0, ChannelSelectionOptions{
				RequestPath: test.path,
				Path:        test.path,
			})
			require.NoError(t, err)
			if test.wantNoHit {
				assert.Nil(t, selected)
				return
			}
			require.NotNil(t, selected)
			assert.Equal(t, test.wantID, selected.Id)
		})
	}
}

func TestAutoDLRequestPathAndModelMatrixWithMemoryCache(t *testing.T) {
	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = true
	ClearChannelCacheForTest()
	t.Cleanup(func() {
		ClearChannelCacheForTest()
		common.MemoryCacheEnabled = previousMemoryCacheEnabled
	})

	priority := int64(10)
	weight := uint(100)
	autoDL := &Channel{Id: 701, Type: constant.ChannelTypeAutoDL, Status: common.ChannelStatusEnabled, Weight: &weight, Priority: &priority}
	openAI := &Channel{Id: 702, Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Weight: &weight, Priority: &priority}
	SetChannelCacheForTest(map[int]*Channel{701: autoDL, 702: openAI}, map[string]map[string][]int{
		"default": {
			"MiniMax-H3":   {701, 702},
			"indextts2-v1": {701, 702},
			"tts-1":        {701, 702},
		},
	})

	for _, test := range autoDLSelectionMatrix {
		t.Run(test.name, func(t *testing.T) {
			selected, err := GetRandomSatisfiedChannelWithOptions("default", test.model, 0, ChannelSelectionOptions{
				RequestPath: test.path,
				Path:        test.path,
			})
			require.NoError(t, err)
			if test.wantNoHit {
				assert.Nil(t, selected)
				return
			}
			require.NotNil(t, selected)
			assert.Equal(t, test.wantID, selected.Id)
		})
	}
}
