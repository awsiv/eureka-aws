package synccatalog

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	_pprof "runtime/pprof"
	"strconv"
	"sync"

	_e "github.com/ArthurHlt/go-eureka-client/eureka"
	sd "github.com/aws/aws-sdk-go-v2/service/servicediscovery"
	"github.com/awsiv/eureka-aws/catalog"
	"github.com/awsiv/eureka-aws/subcommand"
	"github.com/hashicorp/consul/command/flags"
	"github.com/mitchellh/cli"
)

const DefaultPollInterval = "30s"

// Command is the command for syncing the A
type Command struct {
	UI cli.Ui

	flags                   *flag.FlagSet
	http                    *flags.HTTPFlags
	flagToEureka            bool
	flagToAWS               bool
	flagAWSNamespaceID      string
	flagAWSServicePrefix    string
	flagAWSPollInterval     string
	flagAWSDNSTTL           int64
	flagEurekaServicePrefix string
	flagEurekaDomain        string

	once sync.Once
	help string
}

func (c *Command) init() {
	c.flags = flag.NewFlagSet("", flag.ContinueOnError)
	c.flags.BoolVar(&c.flagToEureka, "to-eureka", false,
		"If true, AWS services will be synced to Eureka. (Defaults to false)")
	c.flags.BoolVar(&c.flagToAWS, "to-aws", true,
		"If true, Eureka services will be synced to AWS. (Defaults to false)")
	c.flags.StringVar(&c.flagAWSNamespaceID, "aws-namespace-id",
		"", "The AWS namespace to sync with Eureka services.")
	c.flags.StringVar(&c.flagAWSServicePrefix, "aws-service-prefix",
		"", "A prefix to prepend to all services written to AWS from Eureka. "+
			"If this is not set then services will have no prefix.")
	c.flags.StringVar(&c.flagEurekaServicePrefix, "eureka-service-prefix",
		"", "A prefix to prepend to all services written to Eureka from AWS. "+
			"If this is not set then services will have no prefix.")
	c.flags.StringVar(&c.flagAWSPollInterval, "aws-poll-interval",
		DefaultPollInterval, "The interval between fetching from AWS CloudMap. "+
			"Accepts a sequence of decimal numbers, each with optional "+
			"fraction and a unit suffix, such as \"300ms\", \"10s\", \"1.5m\". "+
			"Defaults to 30s)")
	c.flags.Int64Var(&c.flagAWSDNSTTL, "aws-dns-ttl",
		60, "DNS TTL for services created in AWS CloudMap in seconds. (Defaults to 60)")

	c.http = &flags.HTTPFlags{}
	flags.Merge(c.flags, c.http.ClientFlags())
	flags.Merge(c.flags, c.http.ServerFlags())
	c.help = flags.Usage(help, c.flags)
}

func countGoRoutines(w http.ResponseWriter, r *http.Request) {
	b := []byte(strconv.Itoa(runtime.NumGoroutine()))
	w.Write(b)
}

func getStackTraceHandler() {
	stack := debug.Stack()
	os.Stdout.Write(stack)
	_pprof.Lookup("goroutine").WriteTo(os.Stdout, 2)
}

func (c *Command) Run(args []string) int {
	c.once.Do(c.init)

	if len(c.flags.Args()) > 0 {
		c.UI.Error("Should have no non-flag arguments.")
		return 1
	}

	c.flagAWSNamespaceID = os.Getenv("CLOUDMAP_NAMESPACE")
	if len(c.flagAWSNamespaceID) == 0 {
		c.UI.Error("Environment CLOUDMAP_NAMESPACE is not set")
		return 1
	}

	c.flagEurekaDomain = os.Getenv("EUREKA_DOMAIN")
	if len(c.flagEurekaDomain) == 0 {
		c.UI.Error("Environment EUREKA_DOMAIN is not set")
		return 1
	}

	pollInterval := os.Getenv("POLL_INTERVAL")
	if len(pollInterval) > 0 {
		c.flagAWSPollInterval = pollInterval
	}

	awsDnsTTL, err := strconv.ParseInt(os.Getenv("AWS_DNS_TTL"), 10, 64)
	if err != nil && awsDnsTTL > 0 && awsDnsTTL < 60 {
		c.flagAWSDNSTTL = awsDnsTTL
	}

	//Note:
	//		use credentials_source = EC2InstanceMetadata
	//		https://github.com/aws/aws-sdk-go/issues/1993
	config, err := subcommand.AWSConfig()
	if err != nil {
		c.UI.Error(fmt.Sprintf("Error retrieving AWS session: %s", err))
		return 1
	} else {
		c.UI.Info(fmt.Sprintf("Retrieved AWS session: %v", config.Region))
	}
	awsClient := sd.New(config)

	//return 1
	eurekaClient := _e.NewClient([]string{
		c.flagEurekaDomain,
	})
	if eurekaClient == nil {
		c.UI.Error(fmt.Sprintf("Error connecting to Eureka agent: %s", err))
		return 1
	}

	c.UI.Info(fmt.Sprintf("Polling Interval = %s", c.flagAWSPollInterval))
	c.UI.Info(fmt.Sprintf("Namespace ID = %s", c.flagAWSNamespaceID))
	c.UI.Info(fmt.Sprintf("DNS TTL = %d", c.flagAWSDNSTTL))
	c.UI.Info(fmt.Sprintf("Eureka domain = %s", c.flagEurekaDomain))

	stop := make(chan struct{})
	stopped := make(chan struct{})
	go catalog.Sync(
		c.flagToAWS, c.flagToEureka, c.flagAWSNamespaceID,
		c.flagEurekaServicePrefix, c.flagAWSServicePrefix,
		c.flagAWSPollInterval, c.flagAWSDNSTTL, c.getStaleWithDefaultTrue(),
		awsClient, eurekaClient,
		stop, stopped,
	)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)

	select {
	// Unexpected failure
	case <-stopped:
		return 1
	case <-sigCh:
		c.UI.Info("shutting down...")
		close(stop)
		<-stopped
	}

	return 0
}

func (c *Command) getStaleWithDefaultTrue() bool {
	stale := true
	c.flags.Visit(func(f *flag.Flag) {
		if f.Name == "stale" {
			stale = c.http.Stale()
			return
		}
	})
	return stale
}

func (c *Command) Synopsis() string { return synopsis }
func (c *Command) Help() string {
	c.once.Do(c.init)
	return c.help
}

const synopsis = "Sync AWS services and Eureka services."
const help = `
Usage: eureka-aws sync-catalog [options]

  Sync AWS services, and more with the Eureka service catalog.
  This enables AWS services to discover and communicate with external
  services, and allows external services to discover and communicate with
  AWS services.

`
