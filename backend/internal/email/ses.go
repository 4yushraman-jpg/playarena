package email

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	"github.com/aws/aws-sdk-go-v2/service/sesv2/types"
)

type sesProvider struct {
	client      *sesv2.Client
	fromAddress string
	fromName    string
}

// newSESProviderImpl constructs an SES v2 provider. When accessKey and
// secretKey are empty the default AWS credential chain is used (IAM role,
// environment variables, shared credentials file).
func newSESProviderImpl(region, accessKey, secretKey, fromAddr, fromName string) (Provider, error) {
	opts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(region),
	}
	if accessKey != "" && secretKey != "" {
		opts = append(opts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(accessKey, secretKey, ""),
		))
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(), opts...)
	if err != nil {
		return nil, fmt.Errorf("email: SES load AWS config: %w", err)
	}
	return &sesProvider{
		client:      sesv2.NewFromConfig(awsCfg),
		fromAddress: fromAddr,
		fromName:    fromName,
	}, nil
}

func (p *sesProvider) Send(ctx context.Context, msg Message) error {
	from := p.fromAddress
	if p.fromName != "" {
		from = fmt.Sprintf("%s <%s>", p.fromName, p.fromAddress)
	}

	input := &sesv2.SendEmailInput{
		FromEmailAddress: aws.String(from),
		Destination: &types.Destination{
			ToAddresses: []string{msg.To},
		},
		Content: &types.EmailContent{
			Simple: &types.Message{
				Subject: &types.Content{
					Data:    aws.String(msg.Subject),
					Charset: aws.String("UTF-8"),
				},
				Body: &types.Body{
					Text: &types.Content{
						Data:    aws.String(msg.TextBody),
						Charset: aws.String("UTF-8"),
					},
				},
			},
		},
	}

	if msg.HTMLBody != "" {
		input.Content.Simple.Body.Html = &types.Content{
			Data:    aws.String(msg.HTMLBody),
			Charset: aws.String("UTF-8"),
		}
	}

	if _, err := p.client.SendEmail(ctx, input); err != nil {
		return fmt.Errorf("email: SES send: %w", err)
	}
	return nil
}
