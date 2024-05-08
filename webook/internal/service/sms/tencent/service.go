package tencent

import (
	"context"
	"errors"
	"fmt"
	"github.com/ecodeclub/ekit"
	"github.com/ecodeclub/ekit/slice"
	sms "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/sms/v20210111"
	"github.com/wsqigo/basic-go/webook/pkg/limiter"
)

type Service struct {
	client   *sms.Client
	appId    *string
	signName *string
	limiter  limiter.Limiter
}

func (s *Service) Send(ctx context.Context, tplId string, args []string, numbers ...string) error {
	limited, err := s.limiter.Limit(ctx, "tencent-sms-service")
	if err != nil {
		return err
	}
	if limited {
		return errors.New("触发了限流")
	}
	request := sms.NewSendSmsRequest()
	request.SetContext(ctx)
	request.SmsSdkAppId = s.appId
	request.SignName = s.signName
	request.TemplateId = ekit.ToPtr[string](tplId)
	request.TemplateParamSet = s.toPtrSlice(args)
	request.PhoneNumberSet = s.toPtrSlice(numbers)

	response, err := s.client.SendSms(request)
	// 处理异常
	if err != nil {
		return err
	}
	for _, statusPtr := range response.Response.SendStatusSet {
		if statusPtr == nil {
			// 不可能来这里
			continue
		}
		status := *statusPtr
		if status.Code == nil || *(status.Code) != "Ok" {
			// 发送失败
			return fmt.Errorf("发送短信失败 code: %s, msg: %s", *status.Code, *status.Message)
		}
	}
	return nil
}

func (s *Service) toPtrSlice(data []string) []*string {
	return slice.Map[string, *string](data, func(idx int, src string) *string {
		return &src
	})
}

func NewService(client *sms.Client, appId string, signName string, l limiter.Limiter) *Service {
	return &Service{
		limiter:  l,
		client:   client,
		appId:    &appId,
		signName: &signName,
	}
}
