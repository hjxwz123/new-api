package aws

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestDoAwsClientRequest_AppliesRuntimeHeaderOverrideToAnthropicBeta(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	info := &relaycommon.RelayInfo{
		OriginModelName:           "claude-3-5-sonnet-20240620",
		IsStream:                  false,
		UseRuntimeHeadersOverride: true,
		RuntimeHeadersOverride: map[string]any{
			"anthropic-beta": "computer-use-2025-01-24",
		},
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiKey:            "access-key|secret-key|us-east-1",
			UpstreamModelName: "claude-3-5-sonnet-20240620",
		},
	}

	requestBody := bytes.NewBufferString(`{"messages":[{"role":"user","content":"hello"}],"max_tokens":128}`)
	adaptor := &Adaptor{}

	_, err := doAwsClientRequest(ctx, info, adaptor, requestBody)
	require.NoError(t, err)

	awsReq, ok := adaptor.AwsReq.(*bedrockruntime.InvokeModelInput)
	require.True(t, ok)

	var payload map[string]any
	require.NoError(t, common.Unmarshal(awsReq.Body, &payload))

	anthropicBeta, exists := payload["anthropic_beta"]
	require.True(t, exists)

	values, ok := anthropicBeta.([]any)
	require.True(t, ok)
	require.Equal(t, []any{"computer-use-2025-01-24"}, values)
}

func TestAwsOpenAIResponsesModelMapping(t *testing.T) {
	t.Parallel()

	require.Equal(t, "openai.gpt-5.4", getAwsModelID("gpt-5.4"))
	require.Equal(t, "openai.gpt-5.5", getAwsModelID("gpt-5.5"))
	require.True(t, isAwsOpenAIResponsesRequestModel("gpt-5.4"))
	require.True(t, isAwsOpenAIResponsesRequestModel("gpt-5.5"))
	require.False(t, isAwsOpenAIResponsesRequestModel("gpt-5.4-pro"))
}

func TestAwsOpenAIResponsesRequestUsesBedrockMantle(t *testing.T) {
	t.Parallel()

	adaptor := &Adaptor{}
	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeResponses,
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiKey:            "bedrock-api-key|us-west-2",
			UpstreamModelName: "gpt-5.4",
		},
	}

	converted, err := adaptor.ConvertOpenAIResponsesRequest(nil, info, dto.OpenAIResponsesRequest{
		Model: "gpt-5.4",
		Input: []byte(`"hello"`),
	})
	require.NoError(t, err)

	request, ok := converted.(dto.OpenAIResponsesRequest)
	require.True(t, ok)
	require.Equal(t, "openai.gpt-5.4", request.Model)
	require.Equal(t, "openai.gpt-5.4", info.UpstreamModelName)

	url, err := adaptor.GetRequestURL(info)
	require.NoError(t, err)
	require.Equal(t, "https://bedrock-mantle.us-west-2.api.aws/openai/v1/responses", url)
	require.Equal(t, ClientModeOpenAIResponses, adaptor.ClientMode)
}

func TestAwsOpenAIResponsesHeaderUsesApiKeyOnly(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	ctx.Request.Header.Set("Content-Type", "application/json")

	adaptor := &Adaptor{ClientMode: ClientModeOpenAIResponses}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiKey: "bedrock-api-key|us-east-2",
		},
	}

	header := http.Header{}
	require.NoError(t, adaptor.SetupRequestHeader(ctx, &header, info))
	require.Equal(t, "Bearer bedrock-api-key", header.Get("Authorization"))
	require.Equal(t, "application/json", header.Get("Content-Type"))
}

func TestAwsOpenAIResponsesRejectsChatCompletionsURL(t *testing.T) {
	t.Parallel()

	adaptor := &Adaptor{}
	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeChatCompletions,
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiKey:            "bedrock-api-key|us-east-2",
			UpstreamModelName: "gpt-5.5",
		},
	}

	_, err := adaptor.GetRequestURL(info)
	require.Error(t, err)
	require.Contains(t, err.Error(), "only support /v1/responses")
}

func TestAwsApiKeyClaudeURLAndHeader(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	ctx.Request.Header.Set("Content-Type", "application/json")

	adaptor := &Adaptor{}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiKey:            "bedrock-api-key|us-east-1",
			UpstreamModelName: "claude-3-5-sonnet-20240620",
			ChannelOtherSettings: dto.ChannelOtherSettings{
				AwsKeyType: dto.AwsKeyTypeApiKey,
			},
		},
	}

	url, err := adaptor.GetRequestURL(info)
	require.NoError(t, err)
	require.Equal(t, "https://bedrock-runtime.us-east-1.amazonaws.com/model/anthropic.claude-3-5-sonnet-20240620-v1:0/converse", url)

	header := http.Header{}
	require.NoError(t, adaptor.SetupRequestHeader(ctx, &header, info))
	require.Equal(t, "Bearer bedrock-api-key", header.Get("Authorization"))
	require.Equal(t, "application/json", header.Get("Content-Type"))
}

func TestRewriteAwsOpenAIResponsesPassthroughBodyMapsModel(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{"model":"gpt-5.5","input":"hello","store":false}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gpt-5.5",
		},
	}

	bodyReader, closer, err := rewriteAwsOpenAIResponsesPassthroughBody(ctx, info)
	require.NoError(t, err)
	defer closer.Close()

	body, err := io.ReadAll(bodyReader)
	require.NoError(t, err)

	var payload map[string]any
	require.NoError(t, common.Unmarshal(body, &payload))
	require.Equal(t, "openai.gpt-5.5", payload["model"])
	require.Equal(t, "hello", payload["input"])
	require.Equal(t, false, payload["store"])
	require.Equal(t, "openai.gpt-5.5", info.UpstreamModelName)
	require.Equal(t, int64(len(body)), info.UpstreamRequestBodySize)
}
