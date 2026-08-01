package relay

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel/advancedcustom"
	"github.com/QuantumNous/new-api/relay/channel/kie"
	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/stretchr/testify/assert"
)

func TestGetImageAdaptorSelectsKIEByRoutingProtocol(t *testing.T) {
	t.Parallel()

	info := &relaycommon.RelayInfo{
		ImageRoutingProtocol: dto.ImageRoutingProtocolKIEJobs,
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiType:     constant.APITypeAdvancedCustom,
			ChannelType: constant.ChannelTypeAdvancedCustom,
		},
	}
	assert.IsType(t, &kie.Adaptor{}, GetImageAdaptor(info))

	info.ImageRoutingProtocol = dto.ImageRoutingProtocolAdapter
	assert.IsType(t, &advancedcustom.Adaptor{}, GetImageAdaptor(info))
}
