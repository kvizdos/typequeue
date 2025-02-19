package typequeue_helpers_test

import (
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/kvizdos/typequeue/pkg/typequeue_helpers"
	"github.com/stretchr/testify/assert"
)

// fakeSSMClient is a fake implementation of typequeue_helpers.SSMClient.
type fakeSSMClient struct {
	Output *ssm.GetParameterOutput
	Err    error
}

func (f *fakeSSMClient) GetParameter(input *ssm.GetParameterInput) (*ssm.GetParameterOutput, error) {
	return f.Output, f.Err
}

// Test GetTargetURL success when no Environments are configured.
func TestGetTargetURL_NoEnvironments_Success(t *testing.T) {
	expectedValue := "https://example.com/queue"
	output := &ssm.GetParameterOutput{
		Parameter: &ssm.Parameter{
			Value: aws.String(expectedValue),
		},
	}
	client := &fakeSSMClient{Output: output, Err: nil}

	helper := typequeue_helpers.TypeQueueSSMHelper{
		SSM: client,
	}
	got, err := helper.GetTargetURL("my-queue")
	assert.NoError(t, err)
	assert.Equal(t, expectedValue, got)
}

// Test GetTargetURL when Environments is set but CurrentEnv is empty.
func TestGetTargetURL_WithEnvironments_CurrentEnvEmpty(t *testing.T) {
	helper := typequeue_helpers.TypeQueueSSMHelper{
		Environments: map[string]string{"dev": "/project/dev/"},
		CurrentEnv:   "",
		SSM:          &fakeSSMClient{},
	}
	_, err := helper.GetTargetURL("anything")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "CurrentEnv must be set")
}

// Test GetTargetURL when Environments is set but CurrentEnv is invalid.
func TestGetTargetURL_WithEnvironments_InvalidEnv(t *testing.T) {
	helper := typequeue_helpers.TypeQueueSSMHelper{
		Environments: map[string]string{"dev": "/project/dev/"},
		CurrentEnv:   "prod", // not present in map
		SSM:          &fakeSSMClient{},
	}
	_, err := helper.GetTargetURL("anything")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid currentenv")
}

// Test GetTargetURL when SSM.GetParameter returns an error.
func TestGetTargetURL_SSMError(t *testing.T) {
	fakeErr := errors.New("SSM error")
	client := &fakeSSMClient{Output: nil, Err: fakeErr}
	helper := typequeue_helpers.TypeQueueSSMHelper{
		SSM: client,
	}
	_, err := helper.GetTargetURL("queue")
	assert.Error(t, err)
	assert.Equal(t, fakeErr, err)
}

// Test GetTargetURL when SSM.GetParameter returns nil Parameter.
func TestGetTargetURL_NilParameter(t *testing.T) {
	output := &ssm.GetParameterOutput{
		Parameter: nil,
	}
	client := &fakeSSMClient{Output: output, Err: nil}
	helper := typequeue_helpers.TypeQueueSSMHelper{
		SSM: client,
	}
	_, err := helper.GetTargetURL("queue")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found or has no value")
}

// Test GetTargetURL when SSM.GetParameter returns Parameter with nil Value.
func TestGetTargetURL_NilParameterValue(t *testing.T) {
	output := &ssm.GetParameterOutput{
		Parameter: &ssm.Parameter{
			Value: nil,
		},
	}
	client := &fakeSSMClient{Output: output, Err: nil}
	helper := typequeue_helpers.TypeQueueSSMHelper{
		SSM: client,
	}
	_, err := helper.GetTargetURL("queue")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found or has no value")
}

// Test GetTargetURL success when Environments is configured.
// The helper should prefix the targetQueue with the environment path.
func TestGetTargetURL_WithEnvironments_Success(t *testing.T) {
	expectedValue := "https://example.com/prefixed-queue"
	output := &ssm.GetParameterOutput{
		Parameter: &ssm.Parameter{
			Value: aws.String(expectedValue),
		},
	}
	client := &fakeSSMClient{Output: output, Err: nil}
	envMap := map[string]string{"dev": "/project/dev/"}
	helper := typequeue_helpers.TypeQueueSSMHelper{
		Environments: envMap,
		CurrentEnv:   "dev",
		SSM:          client,
		Logger:       testLogger{}, // using a dummy logger defined below
	}
	// Calling GetTargetURL("queue") should look up "/project/dev/queue"
	got, err := helper.GetTargetURL("queue")
	assert.NoError(t, err)
	assert.Equal(t, expectedValue, got)
}

// A dummy logger implementation for testing.
type testLogger struct{}

func (l testLogger) Debugf(format string, v ...interface{}) {
	// No-op
}
func (l testLogger) Errorf(format string, v ...interface{}) {
	// No-op
}
func (l testLogger) Panicf(format string, v ...interface{}) {
	// No-op
}
