package typequeue_helpers

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/kvizdos/typequeue/pkg/typequeue"
)

type SSMClient interface {
	GetParameter(input *ssm.GetParameterInput) (*ssm.GetParameterOutput, error)
}

type TypeQueueSSMHelper struct {
	AWSRegion string
	// Environments can be used to automatically
	// append an environment prefix to target queues,
	// e.g. targetQueue = "test" can turn into
	// "/project/dev/test" or "/project/prod/test"
	Environments map[string]string
	// Must be set when using Environments
	CurrentEnv string

	Logger typequeue.Logger

	SSM SSMClient
}

func (t *TypeQueueSSMHelper) Connect() error {
	if t.SSM != nil {
		return nil // don't reconnect -- very helpful in Lambdaland
	}

	if t.AWSRegion == "" {
		return fmt.Errorf("typequeue_helpers.TypeQueueSSMHelper: AWSRegion must be set")
	}
	if conn, err := ConnectToSSM(t.AWSRegion); err == nil {
		t.SSM = conn
	} else {
		return fmt.Errorf("typequeue_helpers.TypeQueueSSMHelper: failed to connect to SSM: %s", err.Error())
	}

	return nil
}

func (t TypeQueueSSMHelper) GetTargetURL(targetQueue string) (string, error) {
	if t.Environments != nil {
		if t.CurrentEnv == "" {
			return "", fmt.Errorf("typequeue_helpers.TypeQueueSSMHelper: CurrentEnv must be set when using environments")
		}

		pathPrefix, ok := t.Environments[t.CurrentEnv]

		if !ok {
			return "", fmt.Errorf("typequeue_helpers.TypeQueueSSMHelper: invalid currentenv not present within environments: \"%s\"", t.CurrentEnv)
		}

		targetQueue = fmt.Sprintf("%s%s", pathPrefix, targetQueue)
	}

	if t.Logger != nil {
		t.Logger.Debugf("typequeue_helpers.TypeQueueSSMHelper: getting SSM parameter \"%s\"", targetQueue)
	}

	results, err := t.SSM.GetParameter(&ssm.GetParameterInput{
		Name: aws.String(targetQueue),
	})

	if err != nil {
		return "", err
	}

	if results.Parameter == nil || results.Parameter.Value == nil {
		return "", fmt.Errorf("typequeue_helpers.TypeQueueSSMHelper: parameter \"%s\" not found or has no value", targetQueue)
	}

	return *results.Parameter.Value, nil
}
