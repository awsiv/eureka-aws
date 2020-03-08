package catalog

import (
	"fmt"
	"sync"
	"time"

	e "github.com/ArthurHlt/go-eureka-client/eureka"
	"github.com/hashicorp/go-hclog"
)

// instance info:
//  https://github.com/ArthurHlt/go-eureka-client/blob/3b8dfe04ec6ca280d50f96356f765edb845a00e4/eureka/requests.go#L45-L67
type eureka struct {
	client       *e.Client
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

/*
const (
	ConsulAWSNodeName = "consul-aws"
	WaitTime          = 10
)
*/

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
			//create := onlyInFirst(e.getServices(), aws.getServices())
			//count := aws.create(create)
			//if count > 0 {
			count := 0
			aws.log.Info("created", "count", fmt.Sprintf("%d", count))
			//}

			//remove := onlyInFirst(aws.getServices(), e.getServices())
			//count = aws.remove(remove)
			//if count > 0 {
			aws.log.Info("removed", "count", fmt.Sprintf("%d", count))
			//}
		case <-stop:
			return
		}
	}
}

func (e *eureka) transformNodes(cnodes []e.InstanceInfo) map[string]map[int]node {
	nodes := map[string]map[int]node{}
	for _, n := range cnodes {
		address := n.HostName
		if len(address) == 0 {
			address = n.IpAddr
		}
		if nodes[address] == nil {
			nodes[address] = map[int]node{}
		}
		ports := nodes[address]
		ports[n.Port.Port] = node{port: n.Port.Port, host: address, eurekaID: n.App, awsID: n.App, attributes: n.Metadata.Map}
		nodes[address] = ports
	}
	return nodes
}

// read nodes from eureka
func (e *eureka) fetchNodes(service string) ([]e.InstanceInfo, error) {
	//opts := &api.QueryOptions{AllowStale: e.stale}
	app, err := e.client.GetApplication(service)
	//.Service(service, "", opts)
	if err != nil {
		return nil, fmt.Errorf("error querying Service nodes, will retry: %s", err)
	}
	return app.Instances, err
}

func (e *eureka) transformHealth(ehealths []e.InstanceInfo) map[string]health {
	healths := map[string]health{}
	for _, h := range ehealths {
		switch h.Status {
		case "UP":
			healths[h.InstanceID] = "HEALTHY"
		case "DOWN":
		case "STARTING":
		case "OUT_OF_SERVICE":
			healths[h.InstanceID] = "UNHEALTHY"
		default:
			healths[h.InstanceID] = "UNKNOWN"
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

func (e *eureka) fetchServices() (*e.Applications, error) {
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
	fmt.Printf("services: %v", services)
	return nil
}

func (e *eureka) transformServices(apps *e.Applications) map[string]service {
	services := make(map[string]service, len(apps.Applications))
	for _, v := range apps.Applications {
		// bishwa
		if v.Name == "BABAR" {
			s := service{id: v.Name, name: v.Name, eurekaID: v.Name}
			/*
				if s.fromAWS {
					s.name = strings.TrimPrefix(v.Name, e.awsPrefix)
				}
			*/

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
			continue
		}
	}
}
