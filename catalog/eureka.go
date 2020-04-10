package catalog

import (
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
	"runtime/pprof"
	"sync"
	"time"

	_e "github.com/ArthurHlt/go-eureka-client/eureka"
	"github.com/hashicorp/go-hclog"
)

const (
	EurekaAWSNodeName = "eureka-aws"
	WaitTime          = 10
)

// instance info:
//  https://github.com/ArthurHlt/go-eureka-client/blob/3b8dfe04ec6ca280d50f96356f765edb845a00e4/eureka/requests.go#L45-L67
type eureka struct {
	client       *_e.Client
	log          hclog.Logger
	eurekaPrefix string
	awsPrefix    string
	services     map[string]service
	trigger      chan bool
	toAWS        bool
	stale        bool
	lock         sync.RWMutex
	pullInterval time.Duration
}

func (e *eureka) getServices() map[string]service {
	e.lock.RLock()
	copy := e.services
	e.lock.RUnlock()
	return copy
}

func (e *eureka) getService(name string) (service, bool) {
	e.lock.RLock()
	copy, ok := e.services[name]
	e.lock.RUnlock()
	return copy, ok
}

func (e *eureka) getAWSID(name, host string, port int) (string, bool) {
	if n, ok := e.getNode(name, host, port); ok {
		return n.awsID, true
	}
	return "", false
}

func (e *eureka) getNode(name, host string, port int) (node, bool) {
	e.lock.RLock()
	copy, ok := e.services[name]
	e.lock.RUnlock()
	if ok {
		if nodes, ok := copy.nodes[host]; ok {
			for _, n := range nodes {
				if n.port == port {
					return n, true
				}
			}
		}
	}
	return node{}, false
}

func (e *eureka) getNodeForAWSID(name, id string) (node, bool) {
	e.lock.RLock()
	copy, ok := e.services[name]
	e.lock.RUnlock()
	if !ok {
		return node{}, ok
	}

	for _, nodes := range copy.nodes {
		for _, n := range nodes {
			if n.awsID == id {
				return n, true
			}
		}
	}
	return node{}, false
}

func (e *eureka) setServices(services map[string]service) {
	e.lock.Lock()
	e.services = services
	e.lock.Unlock()
}

func (e *eureka) setNode(k, h string, p int, n node) {
	e.lock.Lock()
	if s, ok := e.services[k]; ok {
		nodes := s.nodes
		if nodes == nil {
			nodes = map[string]map[int]node{}
		}
		ports := nodes[h]
		if ports == nil {
			ports = map[int]node{}
		}
		ports[p] = n
		nodes[h] = ports
		s.nodes = nodes
		e.services[k] = s
	}
	e.lock.Unlock()
}

func (e *eureka) sync(aws *aws, stop, stopped chan struct{}) {
	defer close(stopped)
	for {
		select {
		case <-e.trigger:
			if !e.toAWS {
				continue
			}
			// todo: enable this once everything working
			create := onlyInFirst(e.getServices(), aws.getServices())
			count := aws.create(create)
			if count > 0 {
				count := 0
				e.log.Info("created", "count", fmt.Sprintf("%d", count))
			}

			remove := onlyInFirst(aws.getServices(), e.getServices())
			//e.log.Info("sync()", "aws", aws.getServices(), "eureka", e.getServices())
			count = aws.remove(remove)
			if count > 0 {
				e.log.Info("removed", "count", fmt.Sprintf("%d", count))
			}
		case <-stop:
			e.log.Info("sync()", "stopped", 1)
			return
		}
	}
}

func (e *eureka) transformNodes(cnodes []_e.InstanceInfo) map[string]map[int]node {
	nodes := map[string]map[int]node{}
	attributes := make(map[string]string)

	for _, n := range cnodes {
		privateip := n.DataCenterInfo.Metadata.LocalIpv4

		if nodes[privateip] == nil {
			nodes[privateip] = map[int]node{}
		}

		ports := nodes[privateip]

		attributes["public-ipv4"] = n.DataCenterInfo.Metadata.PublicIpv4
		attributes["local-ipv4"] = n.DataCenterInfo.Metadata.LocalIpv4
		attributes["public-hostname"] = n.DataCenterInfo.Metadata.PublicHostname
		attributes["local-hostname"] = n.DataCenterInfo.Metadata.LocalHostname
		attributes["availability-zone"] = n.DataCenterInfo.Metadata.AvailabilityZone
		attributes["homePageUrl"] = n.HomePageUrl
		attributes["statusPageUrl"] = n.StatusPageUrl
		attributes["healthCheckUrl"] = n.HealthCheckUrl

		ports[n.Port.Port] = node{port: n.Port.Port, host: attributes["local-ipv4"], eurekaID: n.App, awsID: n.App, attributes: attributes, instanceID: n.DataCenterInfo.Metadata.InstanceId}
		nodes[privateip] = ports
		//e.log.Debug("transformNodes()", "port", n.Port.Port, "ipAddr", n.IpAddr, "attributes", attributes, "instanceId", n.DataCenterInfo.Metadata.InstanceId)
	}
	return nodes
}

// read nodes from eureka
func (e *eureka) fetchNodes(service string) ([]_e.InstanceInfo, error) {
	app, err := e.client.GetApplication(service)
	if err != nil {
		return nil, fmt.Errorf("error querying Service nodes, will retry: %s", err)
	}
	return app.Instances, err
}

func (e *eureka) transformHealth(ehealths []_e.InstanceInfo) map[string]health {
	healths := map[string]health{}

	for _, h := range ehealths {
		instanceId := h.DataCenterInfo.Metadata.InstanceId
		//e.log.Info("transformHealth()", "instanceID", instanceId, "status", h.Status)

		switch h.Status {
		case "UP":
			healths[instanceId] = "HEALTHY"
		case "DOWN":
		case "STARTING":
		case "OUT_OF_SERVICE":
			healths[instanceId] = "UNHEALTHY"
		default:
			healths[instanceId] = "UNKNOWN"
		}
	}
	return healths
}

/*
* eureka doesn't provide this endpoint
func (c *eureka) fetchHealth(name string) (api.HealthChecks, error) {
	opts := &api.QueryOptions{AllowStale: c.stale}
	status, _, err := c.client.Health().Checks(name, opts)
	if err != nil {
		return nil, fmt.Errorf("error querying health, will retry: %s", err)
	}
	return status, nil
}
*/

func (e *eureka) fetchServices() (*_e.Applications, error) {
	apps, err := e.client.GetApplications()
	if err != nil {
		return apps, err
	}
	return apps, nil
}

func (e *eureka) fetch() error {
	apps, err := e.fetchServices()
	if err != nil {
		return fmt.Errorf("error fetching services: %s", err)
	}
	services := e.transformServices(apps)

	e.setServices(services)
	//fmt.Printf("services: %v", services)
	return nil
}

func (e *eureka) transformServices(apps *_e.Applications) map[string]service {
	services := make(map[string]service, len(apps.Applications))
	for _, v := range apps.Applications {
		//TODO: bishwa
		if v.Name == "CORNELIUS" || v.Name == "s1" || v.Name == "s2" || v.Name == "s3" {
			s := service{id: v.Name, name: v.Name, eurekaID: v.Name, fromEureka: true}
			/*
				if s.fromAWS {
					s.name = strings.TrimPrefix(v.Name, e.awsPrefix)
				}
			*/

			//e.log.Info("transformServices()", "serviceName", v.Name)
			s.nodes = e.transformNodes(v.Instances)
			s.healths = e.transformHealth(v.Instances)

			services[s.name] = s
		}
	}
	return services
}

func (e *eureka) rekeyHealths(name string, healths map[string]health) map[string]health {
	rekeyed := map[string]health{}
	for id, h := range healths {
		host, port := hostPortFromID(id)
		if awsID, ok := e.getAWSID(name, host, port); ok {
			rekeyed[awsID] = h
		}
	}
	return rekeyed
}

func countGoRoutines() int {
	return runtime.NumGoroutine()
}

func getStackTraceHandler() {
	stack := debug.Stack()
	os.Stdout.Write(stack)
	pprof.Lookup("goroutine").WriteTo(os.Stdout, 2)
}

func (e *eureka) fetchIndefinetely(stop, stopped chan struct{}) {
	defer close(stopped)

	for {
		err := e.fetch()
		if err != nil {
			e.log.Error("error fetching", "error", err.Error())
		} else {
			e.trigger <- true
		}
		select {
		case <-stop:
			return
		case <-time.After(e.pullInterval):
			e.log.Info("goroutine", "count", countGoRoutines())
			//getStackTraceHandler()
			continue
		}
	}
}
