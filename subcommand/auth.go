package subcommand

import (
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/external"
)

func AWSConfig() (aws.Config, error) {
	// TODO: read from ENV, use ec2metadata
	return external.LoadDefaultAWSConfig()
}
