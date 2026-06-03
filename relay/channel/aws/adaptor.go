package aws

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/claude"
	"github.com/QuantumNous/new-api/relay/channel/openai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/pkg/errors"

	"github.com/gin-gonic/gin"
)

type ClientMode int

const (
	ClientModeApiKey ClientMode = iota + 1
	ClientModeAKSK
	ClientModeOpenAIResponses
)

type Adaptor struct {
	ClientMode ClientMode
	AwsClient  *bedrockruntime.Client
	AwsModelId string
	AwsReq     any
	IsNova     bool
}

func (a *Adaptor) ConvertGeminiRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ClaudeRequest) (any, error) {
	for i, message := range request.Messages {
		updated := false
		if !message.IsStringContent() {
			content, err := message.ParseContent()
			if err != nil {
				return nil, errors.Wrap(err, "failed to parse message content")
			}
			for i2, mediaMessage := range content {
				if mediaMessage.Source != nil {
					if mediaMessage.Source.Type == "url" {
						// 使用统一的文件服务获取图片数据
						source := types.NewURLFileSource(mediaMessage.Source.Url)
						base64Data, mimeType, err := service.GetBase64Data(c, source, "formatting image for Claude")
						if err != nil {
							return nil, fmt.Errorf("get file base64 from url failed: %s", err.Error())
						}
						mediaMessage.Source.MediaType = mimeType
						mediaMessage.Source.Data = base64Data
						mediaMessage.Source.Url = ""
						mediaMessage.Source.Type = "base64"
						content[i2] = mediaMessage
						updated = true
					}
				}
			}
			if updated {
				message.SetContent(content)
			}
		}
		if updated {
			request.Messages[i] = message
		}
	}
	return request, nil
}

func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	awsModelId := getAwsModelID(info.UpstreamModelName)
	if isAwsOpenAIResponsesModel(awsModelId) {
		a.ClientMode = ClientModeOpenAIResponses
		if info.RelayMode != relayconstant.RelayModeResponses {
			return "", errors.New("aws openai gpt-5.4/gpt-5.5 models only support /v1/responses")
		}
		awsSecret := strings.Split(info.ApiKey, "|")
		if len(awsSecret) != 2 {
			return "", errors.New("invalid aws api key for bedrock-mantle, should be in format of <api-key>|<region>")
		}
		region := strings.TrimSpace(awsSecret[1])
		if region == "" {
			return "", errors.New("invalid aws api key for bedrock-mantle, region is required")
		}
		return fmt.Sprintf("https://bedrock-mantle.%s.api.aws/openai/v1/responses", region), nil
	}

	if info.ChannelOtherSettings.AwsKeyType == dto.AwsKeyTypeApiKey {
		a.ClientMode = ClientModeApiKey
		awsSecret := strings.Split(info.ApiKey, "|")
		if len(awsSecret) != 2 {
			return "", errors.New("invalid aws api key, should be in format of <api-key>|<region>")
		}
		return fmt.Sprintf("https://bedrock-runtime.%s.amazonaws.com/model/%s/converse", awsSecret[1], awsModelId), nil
	}
	a.ClientMode = ClientModeAKSK
	return "", nil
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)
	claude.CommonClaudeHeadersOperation(c, req, info)
	if a.ClientMode == ClientModeApiKey || a.ClientMode == ClientModeOpenAIResponses {
		apiKey := info.ApiKey
		if awsSecret := strings.Split(info.ApiKey, "|"); len(awsSecret) == 2 {
			apiKey = strings.TrimSpace(awsSecret[0])
		}
		req.Set("Authorization", "Bearer "+apiKey)
	}
	return nil
}

func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	if isAwsOpenAIResponsesRequestModel(request.Model) {
		return nil, errors.New("aws openai gpt-5.4/gpt-5.5 models only support /v1/responses")
	}
	// 检查是否为Nova模型
	if isNovaModel(request.Model) {
		novaReq := convertToNovaRequest(request)
		a.IsNova = true
		return novaReq, nil
	}

	// 原有的Claude模型处理逻辑
	claudeReq, err := claude.RequestOpenAI2ClaudeMessage(c, *request)
	if err != nil {
		return nil, errors.Wrap(err, "failed to convert openai request to claude request")
	}
	info.UpstreamModelName = claudeReq.Model
	return claudeReq, err
}

func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, nil
}

func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	if !isAwsOpenAIResponsesRequestModel(request.Model) {
		return nil, errors.New("aws channel only supports /v1/responses for gpt-5.4/gpt-5.5")
	}
	request.Model = getAwsModelID(request.Model)
	info.UpstreamModelName = request.Model
	return request, nil
}

func rewriteAwsOpenAIResponsesPassthroughBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, io.Closer, error) {
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return nil, nil, err
	}
	body, err := storage.Bytes()
	if err != nil {
		return nil, nil, err
	}
	var data map[string]any
	if err := common.Unmarshal(body, &data); err != nil {
		return nil, nil, err
	}
	model, _ := data["model"].(string)
	awsModelId := getAwsModelID(model)
	if !isAwsOpenAIResponsesModel(awsModelId) {
		awsModelId = getAwsModelID(info.UpstreamModelName)
	}
	if !isAwsOpenAIResponsesModel(awsModelId) {
		return nil, nil, errors.New("aws channel only supports /v1/responses for gpt-5.4/gpt-5.5")
	}
	data["model"] = awsModelId
	info.UpstreamModelName = awsModelId

	jsonData, err := common.Marshal(data)
	if err != nil {
		return nil, nil, err
	}
	bodyReader, size, closer, err := relaycommon.NewOutboundJSONBody(jsonData)
	if err != nil {
		return nil, nil, err
	}
	info.UpstreamRequestBodySize = size
	return bodyReader, closer, nil
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	if a.ClientMode == ClientModeOpenAIResponses || isAwsOpenAIResponsesRequestModel(info.UpstreamModelName) {
		if model_setting.GetGlobalSettings().PassThroughRequestEnabled || info.ChannelSetting.PassThroughBodyEnabled {
			bodyReader, closer, err := rewriteAwsOpenAIResponsesPassthroughBody(c, info)
			if err != nil {
				return nil, err
			}
			defer closer.Close()
			requestBody = bodyReader
		}
		return channel.DoApiRequest(a, c, info, requestBody)
	}
	if a.ClientMode == ClientModeApiKey || info.ChannelOtherSettings.AwsKeyType == dto.AwsKeyTypeApiKey {
		return channel.DoApiRequest(a, c, info, requestBody)
	}
	return doAwsClientRequest(c, info, a, requestBody)
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {
	if a.ClientMode == ClientModeOpenAIResponses {
		if info.IsStream {
			usage, err = openai.OaiResponsesStreamHandler(c, info, resp)
		} else {
			usage, err = openai.OaiResponsesHandler(c, info, resp)
		}
	} else if a.ClientMode == ClientModeApiKey {
		claudeAdaptor := claude.Adaptor{}
		usage, err = claudeAdaptor.DoResponse(c, resp, info)
	} else {
		if a.IsNova {
			err, usage = handleNovaRequest(c, info, a)
		} else {
			if info.IsStream {
				err, usage = awsStreamHandler(c, info, a)
			} else {
				err, usage = awsHandler(c, info, a)
			}
		}
	}
	return
}

func (a *Adaptor) GetModelList() (models []string) {
	for n := range awsModelIDMap {
		models = append(models, n)
	}

	return
}

func (a *Adaptor) GetChannelName() string {
	return ChannelName
}
