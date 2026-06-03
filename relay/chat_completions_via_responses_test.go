package relay

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/require"
)

func TestShouldAwsOpenAIModelUseResponses(t *testing.T) {
	t.Parallel()

	require.True(t, shouldAwsOpenAIModelUseResponses(&relaycommon.RelayInfo{
		OriginModelName: "gpt-5.4",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType: constant.ChannelTypeAws,
		},
	}))
	require.True(t, shouldAwsOpenAIModelUseResponses(&relaycommon.RelayInfo{
		OriginModelName: "aws-gpt-5.5-alias",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:       constant.ChannelTypeAws,
			UpstreamModelName: "openai.gpt-5.5",
		},
	}))
	require.False(t, shouldAwsOpenAIModelUseResponses(&relaycommon.RelayInfo{
		OriginModelName: "gpt-5.4-pro",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType: constant.ChannelTypeAws,
		},
	}))
	require.False(t, shouldAwsOpenAIModelUseResponses(&relaycommon.RelayInfo{
		OriginModelName: "gpt-5.4",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType: constant.ChannelTypeOpenAI,
		},
	}))
}
