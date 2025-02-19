package typequeue_helpers_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/stretchr/testify/assert"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/kvizdos/typequeue/pkg/typequeue_helpers"
)

// createLocalStackContainer starts LocalStack with SSM enabled and returns its endpoint.
func createLocalStackContainer(ctx context.Context) (testcontainers.Container, string, error) {
	req := testcontainers.ContainerRequest{
		Image:        "localstack/localstack:latest",
		ExposedPorts: []string{"4566/tcp"},
		Env: map[string]string{
			"SERVICES": "ssm", // enable SSM service
		},
		WaitingFor: wait.ForLog("Ready."),
	}
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, "", err
	}
	host, err := container.Host(ctx)
	if err != nil {
		return nil, "", err
	}
	mappedPort, err := container.MappedPort(ctx, "4566")
	if err != nil {
		return nil, "", err
	}
	endpoint := fmt.Sprintf("http://%s:%s", host, mappedPort.Port())
	return container, endpoint, nil
}

// newSSMSession creates an SSM client pointing at LocalStack.
func newSSMSession(region, endpoint string) (*ssm.SSM, error) {
	sess, err := session.NewSession(&aws.Config{
		Region:      aws.String(region),
		Endpoint:    aws.String(endpoint),
		Credentials: credentials.NewStaticCredentials("test", "test", ""),
	})
	if err != nil {
		return nil, err
	}
	return ssm.New(sess), nil
}

func TestIntegrationTypeQueueSSMHelper_Success(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := context.Background()

	// Start LocalStack with SSM enabled.
	container, endpoint, err := createLocalStackContainer(ctx)
	assert.NoError(t, err, "LocalStack container should start")
	defer container.Terminate(ctx)

	// Create a new SSM client session pointing to LocalStack.
	ssmClient, err := newSSMSession("us-east-1", endpoint)
	assert.NoError(t, err)

	// Create an instance of the helper with the SSM client already injected.
	helper := typequeue_helpers.TypeQueueSSMHelper{
		AWSRegion: "us-east-1",
		SSM:       ssmClient,
	}

	// Put a parameter into SSM.
	paramName := "my-target-queue"
	expectedValue := "https://sqs.us-east-1.amazonaws.com/123456789012/my-target-queue"
	_, err = ssmClient.PutParameter(&ssm.PutParameterInput{
		Name:      aws.String(paramName),
		Value:     aws.String(expectedValue),
		Type:      aws.String("String"),
		Overwrite: aws.Bool(true),
	})
	assert.NoError(t, err, "PutParameter should succeed")

	// Retrieve the parameter using the helper.
	gotValue, err := helper.GetTargetURL(paramName)
	assert.NoError(t, err, "GetTargetURL should succeed")
	assert.Equal(t, expectedValue, gotValue, "Returned parameter value should match expected")
}

func TestIntegrationTypeQueueSSMHelper_Environments_Success(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := context.Background()

	// Start LocalStack with SSM enabled.
	container, endpoint, err := createLocalStackContainer(ctx)
	assert.NoError(t, err, "LocalStack container should start")
	defer container.Terminate(ctx)

	// Create a new SSM client session pointing to LocalStack.
	ssmClient, err := newSSMSession("us-east-1", endpoint)
	assert.NoError(t, err)

	// Define the environment configuration.
	envMap := map[string]string{"dev": "/project/dev/"}
	currentEnv := "dev"

	// Create an instance of the helper with the SSM client already injected and environments configured.
	helper := typequeue_helpers.TypeQueueSSMHelper{
		AWSRegion:    "us-east-1",
		SSM:          ssmClient,
		Environments: envMap,
		CurrentEnv:   currentEnv,
	}

	// The parameter name must include the environment prefix.
	targetQueue := "my-target-queue"
	prefixedName := fmt.Sprintf("%s%s", envMap[currentEnv], targetQueue)
	expectedValue := "https://sqs.us-east-1.amazonaws.com/123456789012/my-target-queue"

	// Put the parameter into SSM using the prefixed name.
	_, err = ssmClient.PutParameter(&ssm.PutParameterInput{
		Name:      aws.String(prefixedName),
		Value:     aws.String(expectedValue),
		Type:      aws.String("String"),
		Overwrite: aws.Bool(true),
	})
	assert.NoError(t, err, "PutParameter should succeed")

	// Retrieve the parameter using the helper (which will add the prefix automatically).
	gotValue, err := helper.GetTargetURL(targetQueue)
	assert.NoError(t, err, "GetTargetURL should succeed")
	assert.Equal(t, expectedValue, gotValue, "Returned parameter value should match expected")
}
