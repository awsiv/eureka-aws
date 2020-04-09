package catalog

import (
	"time"

	e "github.com/ArthurHlt/go-eureka-client/eureka"
	sd "github.com/aws/aws-sdk-go-v2/service/servicediscovery"
	"github.com/hashicorp/go-hclog"
)

// Sync aws->eureka and vice versa.

func Sync(toAWS, toEureka bool, namespaceID, eurekaPrefix, awsPrefix, awsPullInterval string, awsDNSTTL int64, stale bool, awsClient *sd.Client, eurekaClient *e.Client, stop, stopped chan struct{}) {
	defer close(stopped)
	log := hclog.Default().Named("sync")

	pullInterval, err := time.ParseDuration(awsPullInterval)
	if err != nil {
		log.Error("cannot parse aws pull interval", "error", err)
		return
	}

	eureka := eureka{
		client:       eurekaClient,
		log:          hclog.Default().Named("eureka"),
		trigger:      make(chan bool, 1),
		eurekaPrefix: eurekaPrefix,
		awsPrefix:    awsPrefix,
		toAWS:        toAWS,
		stale:        stale,
		pullInterval: pullInterval,
	}

	aws := aws{
		client:       awsClient,
		log:          hclog.Default().Named("aws"),
		trigger:      make(chan bool, 1),
		eurekaPrefix: eurekaPrefix,
		awsPrefix:    awsPrefix,
		toEureka:     toEureka,
		pullInterval: pullInterval,
		dnsTTL:       awsDNSTTL,
	}

	err = aws.setupNamespace(namespaceID)
	if err != nil {
		log.Error("cannot setup namespace", "namespaceID", namespaceID, "error", err)
		return
	}

	fetchEurekaStop := make(chan struct{})
	fetchEurekaStopped := make(chan struct{})
	fetchAWSStop := make(chan struct{})
	fetchAWSStopped := make(chan struct{})

	go aws.fetchIndefinetely(fetchAWSStop, fetchAWSStopped)
	go eureka.fetchIndefinetely(fetchEurekaStop, fetchEurekaStopped)

	toEurekaStop := make(chan struct{})
	toEurekaStopped := make(chan struct{})
	toAWSStop := make(chan struct{})
	toAWSStopped := make(chan struct{})

	go aws.sync(&eureka, toEurekaStop, toEurekaStopped)
	go eureka.sync(&aws, toAWSStop, toAWSStopped)

	select {
	case <-stop:
		close(toEurekaStop)
		close(toAWSStop)
		close(fetchEurekaStop)
		close(fetchAWSStop)
		<-toEurekaStopped
		<-toAWSStopped
		<-fetchAWSStopped
		<-fetchEurekaStopped
	case <-fetchAWSStopped:
		log.Info("problem with aws fetch. shutting down...")
		close(toEurekaStop)
		close(toAWSStop)
		close(fetchEurekaStop)
		<-toEurekaStopped
		<-toAWSStopped
		<-fetchEurekaStopped
	case <-fetchEurekaStopped:
		log.Info("problem with eureka fetch. shutting down...")
		close(toEurekaStop)
		close(fetchAWSStop)
		close(toAWSStop)
		<-toEurekaStopped
		<-toAWSStopped
		<-fetchAWSStopped

	case <-toEurekaStopped:
		log.Info("problem with eureka sync. shutting down...")
		close(fetchEurekaStop)
		close(toAWSStop)
		close(fetchAWSStop)
		<-toAWSStopped
		<-fetchAWSStopped
		<-fetchEurekaStopped

	case <-toAWSStopped:
		log.Info("problem with aws sync. shutting down...")
		close(toEurekaStop)
		close(fetchEurekaStop)
		close(fetchAWSStop)
		<-toEurekaStopped
		<-fetchEurekaStopped
		<-fetchAWSStopped
	}
}
