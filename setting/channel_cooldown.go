package setting

import "github.com/QuantumNous/new-api/types"

// ChannelCooldownDisabled is an operator override keyed by channel ID. It does
// not enable disabled channels or change authentication, billing or retries.
var channelCooldownDisabled = types.NewRWMap[int, bool]()

func IsChannelCooldownDisabled(channelID int) bool {
	disabled, _ := channelCooldownDisabled.Get(channelID)
	return disabled
}

func UpdateChannelCooldownDisabled(value string) error {
	return types.LoadFromJsonString(channelCooldownDisabled, value)
}

func ChannelCooldownDisabledJSON() string {
	return channelCooldownDisabled.MarshalJSONString()
}
