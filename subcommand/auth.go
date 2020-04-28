package subcommand

import (
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/external"
)

func AWSConfig() (aws.Config, error) {
	// TODO: read from ENV
	return external.LoadDefaultAWSConfig(
		external.WithSharedConfigProfile("tidal-stage"),
		external.WithRegion("us-east-1"),
	)
}
