# Eureka-AWS

Based on `consul-aws`: https://github.com/hashicorp/consul-aws

`eureka-aws` syncs the services in Eureka toAWS Cloudmap Namespace. Eureka services will be created in AWS CloudMap and the other way around. This enables native service discovery across Eureka and AWS CloudMap.

This project is versioned separately from Eureka. Supported Eureka versions for each feature will be noted below. By versioning this project separately, we can iterate on AWS integrations more quickly and release new versions without forcing Eureka users to do a full Eureka upgrade.

## Installation

1. Download a pre-compiled, released version from the [Eureka-AWS releases page][releases].

1. Extract the binary using `unzip` or `tar`.

1. Move the binary into `$PATH`.

To compile from source, please see the instructions in the [contributing section](#contributing).

## Usage

`eureka-aws` can sync from Eureka to AWS CloudMap (`-to-aws`), from AWS CloudMap to Eureka (`-to-eureka`) and both at the same time. No matter which direction is being used `eureka-aws` needs to be connected to Eureka and AWS CloudMap.

In order to help with connecting to a Eureka cluster, `eureka-aws` provides all the flags you might need including the possibility to set an ACL token. `eureka-aws` loads your AWS configuration from `.aws`, from the instance profile and ENV variables - it supports everything provided by the AWS golang sdk.

Apart from that a AWS CloudMap namespace id has to be provided. This is how `eureka-aws` could be invoked to sync both directions:

```shell
$ ./eureka-aws sync-catalog -aws-namespace-id ns-hjrgt3bapp7phzff -to-aws -to-eureka
```

```shell
# Running container with environment variables
export CLOUDMAP_NAMESPACE=ns-zsexdrcft
export EUREKA_DOMAIN=http://<url>/eureka/v2
export POLL_INTERVAL=60s
export AWS_DNS_TTL=30

$ docker run -it -env CLOUDMAP_NAMESPACE -env EUREKA_DOMAIN -env POLL_INTERVAL -env AWS_DNS_TTL  src/eureka-aws:latest sync-catalog

```

## Contributing

To build and install `eureka-aws` locally, Go version 1.11+ is required because this repository uses go modules.
You will also need to install the Docker engine:

- [Docker for Mac](https://docs.docker.com/engine/installation/mac/)
- [Docker for Windows](https://docs.docker.com/engine/installation/windows/)
- [Docker for Linux](https://docs.docker.com/engine/installation/linux/ubuntulinux/)

Clone the repository:

```shell
$ git clone https://github.com/awsiv/eureka-aws.git
```

To compile the `eureka-aws` binary for your local machine:

```shell
$ make dev
```

This will compile the `eureka-aws` binary into `bin/eureka-aws` as well as your `$GOPATH` and run the test suite.

Or run the following to generate all binaries:

```shell
$ make dist
```

To create a docker image with your local changes:

```shell
$ make dev-docker
```
## Testing

If you just want to run the tests:

```shell
$ make test
```

Or to run a specific test in the suite:

```shell
go test ./... -run SomeTestFunction_name
```

**Note:** To run the sync integration tests, you must specify `INTTEST=1` in your environment and [AWS credentials](https://docs.aws.amazon.com/sdk-for-go/v1/developer-guide/configuring-sdk.html#specifying-credentials).

## Compatibility with Eureka

`eureka-aws` supports the current version of Eureka and the version before. At the time of writing this, it means `1.4` and `1.3`.

[releases]: https://releases.hashicorp.com/eureka-aws "Eureka-AWS Releases"
