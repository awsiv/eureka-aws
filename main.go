package main

import (
	"log"
	"os"

	"github.com/awsiv/eureka-aws/version"
	"github.com/mitchellh/cli"
)

func main() {
	c := cli.NewCLI("eureka-aws", version.GetHumanVersion())
	c.Args = os.Args[1:]
	c.Commands = Commands
	c.HelpFunc = helpFunc()

	exitStatus, err := c.Run()
	if err != nil {
		log.Println(err)
	}
	os.Exit(exitStatus)
}
