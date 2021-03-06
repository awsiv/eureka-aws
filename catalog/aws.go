package catalog

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-go/statsd"
	x "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/awserr"
	sd "github.com/aws/aws-sdk-go-v2/service/servicediscovery"
	"github.com/hashicorp/go-hclog"
)

type namespace struct {
	id     string
	name   string
	isHTTP bool
}

const (
	// EurekaAWSTag is used for imported services from AWS
	EurekaAWSTag    = "aws"
	EurekaSourceKey = "external-source"
	EurekaAWSNS     = "external-aws-ns"
	EurekaAWSID     = "external-aws-id"
)

type aws struct {
	lock         sync.RWMutex
	client       *sd.Client
	dd           *statsd.Client
	log          hclog.Logger
	namespace    namespace
	services     map[string]service
	trigger      chan bool
	eurekaPrefix string
	awsPrefix    string
	toEureka     bool
	pullInterval time.Duration
	dnsTTL       int64
}

var awsServiceDescription = "Imported from Eureka"

func (a *aws) sync(eureka *eureka, stop, stopped chan struct{}) {
	defer close(stopped)
	for {
		select {
		case <-a.trigger:
			if !a.toEureka {
				continue
			}
			/*
				// todo: enable this once everything working
				create := onlyInFirst(a.getServices(), eureka.getServices())
				count := eureka.create(create)
				if count > 0 {
					count := 0
					e.log.Info("created", "count", fmt.Sprintf("%d", count))
				}

				remove := onlyInFirst(aws.getServices(), e.getServices())
				//e.log.Info("sync()", "aws", aws.getServices(), "eureka", e.getServices())
				count = aws.remove(remove)
				if count > 0 {
					e.log.Info("removed", "count", fmt.Sprintf("%d", count))
				}*/
		case <-stop:
			a.log.Info("sync()", "stopped", 1)
			return
		}
	}
}

func (a *aws) fetchNamespace(id string) (*sd.Namespace, error) {
	req := a.client.GetNamespaceRequest(&sd.GetNamespaceInput{Id: x.String(id)})
	resp, err := req.Send(context.Background())
	if err != nil {
		return nil, err
	}
	return resp.Namespace, nil
}

func (a *aws) fetchServices() ([]sd.ServiceSummary, error) {
	req := a.client.ListServicesRequest(&sd.ListServicesInput{
		Filters: []sd.ServiceFilter{{
			Name:      sd.ServiceFilterNameNamespaceId,
			Condition: sd.FilterConditionEq,
			Values:    []string{a.namespace.id},
		}},
	})

	resp, err := req.Send(context.Background())
	if err != nil {
		return nil, err
	}
	a.log.Debug("fetchServices()", "resp", resp)
	a.log.Info("fetchServices()", "count", len(resp.Services))

	err = a.dd.Gauge("eureka_aws.sync.aws.services.count",
		float64(len(resp.Services)),
		[]string{"environment:stage-v2"}, 1)

	if err != nil {
		a.log.Error("Unable to post to statsd", "error", err)
	}

	services := resp.Services
	return services, nil
}

func (a *aws) transformServices(awsServices []sd.ServiceSummary) map[string]service {
	services := map[string]service{}
	for _, as := range awsServices {
		s := service{
			id:           *as.Id,
			name:         *as.Name,
			awsID:        *as.Id,
			awsNamespace: a.namespace.id,
		}
		if as.Description != nil && *as.Description == awsServiceDescription {
			s.fromEureka = true
			s.name = strings.TrimPrefix(s.name, a.eurekaPrefix)
		}

		services[s.name] = s
	}
	return services
}

func (a *aws) transformNamespace(awsNamespace *sd.Namespace) namespace {
	namespace := namespace{id: *awsNamespace.Id, name: *awsNamespace.Name}
	if awsNamespace.Type == sd.NamespaceTypeHttp {
		namespace.isHTTP = true
	}
	return namespace
}

func (a *aws) setupNamespace(id string) error {
	namespace, err := a.fetchNamespace(id)
	if err != nil {
		return err
	}
	a.namespace = a.transformNamespace(namespace)
	return nil
}

func (a *aws) fetch() error {
	awsService, err := a.fetchServices()
	if err != nil {
		return err
	}
	services := a.transformServices(awsService)
	for h, s := range services {
		var awsNodes []sd.InstanceSummary
		var err error
		name := s.name
		if s.fromEureka {
			name = a.eurekaPrefix + name
		}
		awsNodes, err = a.discoverNodes(name)
		if err != nil {
			a.log.Error("cannot discover nodes", "error", err)
			continue
		}

		nodes := a.transformNodes(awsNodes)
		//a.log.Info("fetch()", "service", s.name, "nodesCount", length(nodes))
		if len(nodes) == 0 {
			continue
		}
		s.nodes = nodes

		healths, err := a.fetchHealths(s.awsID)
		a.log.Info("fetch()", "healths", healths, "awsID", s.awsID)
		if err != nil {
			a.log.Error("fetch(): cannot fetch healths", "error", err)
		} else {
			if s.fromEureka {
				healths = a.rekeyHealths(s.name, healths)
			}
			s.healths = healths
		}

		services[h] = s
		a.log.Debug("fetch()", "service", s)
	}
	a.setServices(services)
	return nil
}

func (a *aws) getNodeForEurekaID(name, id string) (node, bool) {
	a.lock.RLock()
	copy, ok := a.services[name]
	a.lock.RUnlock()
	if !ok {
		return node{}, ok
	}
	for _, nodes := range copy.nodes {
		for _, n := range nodes {
			if n.eurekaID == id {
				return n, true
			}
		}
	}
	return node{}, false
}

func (a *aws) rekeyHealths(name string, healths map[string]health) map[string]health {
	rekeyed := map[string]health{}
	s, ok := a.getService(name)
	if !ok {
		return nil
	}
	for k, h := range s.healths {
		if n, ok := a.getNodeForEurekaID(name, k); ok {
			rekeyed[id(k, n.host, n.port)] = h
		}
	}
	return rekeyed
}

func statusFromAWS(aws sd.HealthStatus) health {
	var result health
	switch aws {
	case sd.HealthStatusHealthy:
		result = up
	case sd.HealthStatusUnhealthy:
		result = out_of_service
	case sd.HealthStatusUnknown:
		result = unknown
	}
	return result
}

func statusToCustomHealth(h health) sd.CustomHealthStatus {
	var result sd.CustomHealthStatus
	switch h {
	case healthy:
		fallthrough
	case up:
		result = sd.CustomHealthStatusHealthy
		break
	default:
		result = sd.CustomHealthStatusUnhealthy
	}
	return result
}

func (a *aws) fetchHealths(id string) (map[string]health, error) {
	req := a.client.GetInstancesHealthStatusRequest(&sd.GetInstancesHealthStatusInput{
		ServiceId: &id,
	})

	result := map[string]health{}
	resp, err := req.Send(context.Background())
	a.log.Debug("fetchHealths", "resp", resp)

	if err != nil {
		return nil, err
	}

	for id, health := range resp.Status {
		result[id] = statusFromAWS(health)
	}

	return result, nil
}

func (a *aws) transformNodes(awsNodes []sd.InstanceSummary) map[string]map[int]node {
	nodes := map[string]map[int]node{}
	for _, an := range awsNodes {
		h := an.Attributes["AWS_INSTANCE_IPV4"]
		p := 0
		if an.Attributes["AWS_INSTANCE_PORT"] != "" {
			p, _ = strconv.Atoi(an.Attributes["AWS_INSTANCE_PORT"])
		}
		if nodes[h] == nil {
			nodes[h] = map[int]node{}
		}
		n := nodes[h]
		n[p] = node{port: p, host: h, awsID: *an.Id, attributes: an.Attributes}
		nodes[h] = n
	}
	return nodes
}

// TODO: not used anywhere, remove
func (a *aws) fetchNodes(id string) ([]sd.InstanceSummary, error) {
	req := a.client.ListInstancesRequest(&sd.ListInstancesInput{
		ServiceId: &id,
	})

	resp, err := req.Send(context.Background())
	a.log.Debug("fetchNodes", "resp", resp)
	if err != nil {
		a.log.Error("fetchNodes()", "resp", err)
		return nil, err
	}

	nodes := []sd.InstanceSummary{}
	//nodes = append(nodes, resp.Instances)
	/*for p.Next() {
		nodes = append(nodes, p.CurrentPage().Instances...)
	}*/
	return nodes, nil //p.Err()
}

func (a *aws) discoverNodes(name string) ([]sd.InstanceSummary, error) {
	req := a.client.DiscoverInstancesRequest(&sd.DiscoverInstancesInput{
		HealthStatus:  sd.HealthStatusFilterHealthy,
		NamespaceName: x.String(a.namespace.name),
		ServiceName:   x.String(name),
	})
	resp, err := req.Send(context.Background())
	if err != nil {
		return nil, err
	}
	nodes := []sd.InstanceSummary{}
	for _, i := range resp.Instances {
		nodes = append(nodes, sd.InstanceSummary{Id: i.InstanceId, Attributes: i.Attributes})
	}

	a.log.Info("discoverNodes()", "service", name, "count", len(resp.Instances))
	return nodes, nil
}

func (a *aws) getServices() map[string]service {
	a.lock.RLock()
	copy := a.services
	a.lock.RUnlock()
	return copy
}

func (a *aws) getService(name string) (service, bool) {
	a.lock.RLock()
	copy, ok := a.services[name]
	a.lock.RUnlock()
	return copy, ok
}

func (a *aws) setServices(services map[string]service) {
	a.lock.Lock()
	a.services = services
	a.lock.Unlock()
}

//TODO: check if service exists
//      split create service and register nodes
func (a *aws) create(services map[string]service) int {
	wg := sync.WaitGroup{}
	count := 0
	for k, s := range services {
		a.log.Debug("create()", "serviceName", s.name)
		if s.fromAWS {
			continue
		}
		name := a.eurekaPrefix + k
		a.log.Info("create()", "awsServiceName", name, "namespace", a.namespace.id)
		if len(s.awsID) == 0 {
			input := sd.CreateServiceInput{
				Description: &awsServiceDescription,
				Name:        &name,
				NamespaceId: &a.namespace.id,
				HealthCheckCustomConfig: &sd.HealthCheckCustomConfig{
					FailureThreshold: x.Int64(5),
				},
			}

			if !a.namespace.isHTTP {
				input.DnsConfig = &sd.DnsConfig{
					DnsRecords: []sd.DnsRecord{
						{TTL: &a.dnsTTL, Type: sd.RecordTypeSrv},
					},
				}
			}

			req := a.client.CreateServiceRequest(&input)
			resp, err := req.Send(context.Background())
			if err != nil {
				if err, ok := err.(awserr.Error); ok {
					switch err.Code() {
					case sd.ErrCodeServiceAlreadyExists:
						a.log.Info("service already exists", "name", name)
					default:
						a.log.Error("cannot create services in AWS", "error", err)
					}
				} else {
					a.log.Error("cannot create services in AWS", "error", err)
				}
				continue
			}
			s.awsID = *resp.Service.Id

			a.log.Info("Created service:", "name", name, "ns", a.namespace.id, "namespaceID", s.awsID)
			count++
		}

		for h, nodes := range s.nodes {
			for _, n := range nodes {
				wg.Add(1)
				go func(serviceID, name, h string, n node) {
					wg.Done()
					instanceID := n.instanceID //id(serviceID, h, n.port)
					attributes := n.attributes
					if len(n.attributes["local-ipv4"]) > 0 {
						attributes["AWS_INSTANCE_IPV4"] = n.attributes["local-ipv4"]
					} else {
						attributes["AWS_INSTANCE_IPV4"] = n.host
					}

					attributes["AWS_INSTANCE_PORT"] = fmt.Sprintf("%d", n.port)

					req := a.client.RegisterInstanceRequest(&sd.RegisterInstanceInput{
						ServiceId:  &serviceID,
						Attributes: attributes,
						InstanceId: &instanceID,
					})
					_, err := req.Send(context.Background())
					if err != nil {
						a.log.Error("cannot register node", "error", err)
						err := a.dd.Count("eureka_aws.sync.aws.instances.update_error",
							int64(count),
							[]string{}, 1)

						if err != nil {
							a.log.Error("Unable to post to statsd", "error", err)
						}
					} else {
						a.log.Info("Registered node", "ID", instanceID, "service", serviceID, "ip", h, "ns", a.namespace.id)
					}
				}(s.awsID, name, h, n)
			}
			err := a.dd.Count("eureka_aws.sync.aws.services.updated_count",
				1,
				[]string{}, 1)

			if err != nil {
				a.log.Error("Unable to post to statsd", "error", err)
			}
		}
		for instanceID, h := range s.healths {
			wg.Add(1)
			go func(serviceID, instanceID string, h health) {
				defer wg.Done()
				req := a.client.UpdateInstanceCustomHealthStatusRequest(&sd.UpdateInstanceCustomHealthStatusInput{
					ServiceId:  &serviceID,
					InstanceId: &instanceID,
					Status:     statusToCustomHealth(h),
				})
				_, err := req.Send(context.Background())
				if err != nil {
					// Can be ignored for the first time
					err := a.dd.Count("eureka_aws.sync.aws.instances.health_update_error",
						1,
						[]string{}, 1)

					if err != nil {
						a.log.Error("Unable to post to statsd", "error", err)
					}

					a.log.Error("cannot create custom health", "error", err)
				} else {
					err := a.dd.Count("eureka_aws.sync.aws.instances.health_updated",
						1,
						[]string{}, 1)

					if err != nil {
						a.log.Error("Unable to post to statsd", "error", err)
					}

					a.log.Info("custom health status updated", "service", serviceID, "instance", instanceID, "new status", h)
				}
			}(s.awsID, instanceID, h)
		}
		wg.Wait()
	}

	return count
}

func (a *aws) remove(services map[string]service) int {
	wg := sync.WaitGroup{}
	//deletedNodes := []string{}

	for _, s := range services {
		if !s.fromEureka || len(s.awsID) == 0 {
			continue
		}
		for h, nodes := range s.nodes {
			for _, n := range nodes {
				wg.Add(1)
				go func(serviceID, id string) {
					a.log.Info("remove()", "instanceId", n.awsID, "ipv4", h)
					defer wg.Done()
					req := a.client.DeregisterInstanceRequest(&sd.DeregisterInstanceInput{
						ServiceId:  &serviceID,
						InstanceId: &id,
					})
					_, err := req.Send(context.Background())
					if err != nil {
						a.log.Error("cannot remove instance", "error", err)
					} else {
						// TODO:  remove instance from struct
						//delete(nodes, n)
					}
				}(s.awsID, n.awsID)
			}
		}
	}
	wg.Wait()

	count := 0
	for k, s := range services {
		if !s.fromEureka || len(s.awsID) == 0 {
			continue
		}
		origService, _ := a.getService(k)
		if len(s.nodes) < len(origService.nodes) {
			continue
		}
		req := a.client.DeleteServiceRequest(&sd.DeleteServiceInput{
			Id: &s.awsID,
		})
		_, err := req.Send(context.Background())
		if err != nil {
			a.log.Error("cannot remove services", "name", k, "id", s.awsID, "error", err)
		} else {
			// todo remove service from srtuct

			count++
		}
	}
	return count
}

func IsClosed(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
	}

	return false
}

func (a *aws) fetchIndefinetely(stop, stopped chan struct{}) {
	defer close(stopped)

	for {
		err := a.fetch()
		if err != nil {
			a.log.Error("error fetching", "error", err)
		} else {
			a.trigger <- true
		}
		select {
		case <-stop:
			return
		case <-time.After(a.pullInterval):
			continue
		}
	}
}
