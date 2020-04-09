package catalog

import (
	"fmt"
	"strconv"
	"strings"
)

type health string

const (
	up             health = "UP"
	healthy        health = "HEALTHY"
	unhealthy      health = "UNHEALTHY"
	out_of_service health = "OUT_OF_SERVICE"
	unknown        health = ""
)

type service struct {
	id           string
	name         string
	nodes        map[string]map[int]node
	healths      map[string]health
	fromEureka   bool
	fromAWS      bool
	awsID        string
	eurekaID     string
	awsNamespace string
}

type node struct {
	port       int
	host       string
	awsID      string
	eurekaID   string
	instanceID string
	attributes map[string]string
}

func hostPortFromID(checkid string) (string, int) {
	parts := strings.Split(checkid, "_")
	l := len(parts)
	if l >= 2 {
		host := parts[l-2]
		portString := parts[l-1]
		port, _ := strconv.Atoi(portString)
		return host, port
	}
	return "", 0
}

func id(id, host string, port int) string {
	return fmt.Sprintf("%s_%s_%d", id, host, port)
}

func onlyInFirst(servicesA, servicesB map[string]service) map[string]service {
	result := map[string]service{}
	for k, sa := range servicesA {
		if sb, ok := servicesB[k]; !ok {
			result[k] = sa
		} else {
			nodes := map[string]map[int]node{}
			for h, nodesa := range sa.nodes {
				nodesb, ok := sb.nodes[h]
				if !ok {
					nodes[h] = nodesa
					continue
				}
				ports := map[int]node{}
				for p, na := range nodesa {
					if _, ok = nodesb[p]; !ok {
						ports[p] = na
					}
				}
				if len(ports) > 0 {
					nodes[h] = ports
				}
			}
			healths := map[string]health{}
			for k, ha := range sa.healths {
				if hb, ok := sb.healths[k]; !ok {
					healths[k] = ha
				} else {
					if ha != hb {
						healths[k] = ha
					}
				}
			}
			if len(nodes) == 0 && len(healths) == 0 {
				continue
			}
			id := sa.id
			if len(id) == 0 {
				id = sb.id
			}
			name := sa.name
			if len(name) == 0 {
				name = sb.name
			}
			aid := sa.awsID
			if len(aid) == 0 {
				aid = sb.awsID
			}
			cid := sa.eurekaID
			if len(cid) == 0 {
				cid = sb.eurekaID
			}
			ns := sa.awsNamespace
			if len(ns) == 0 {
				ns = sb.awsNamespace
			}
			s := service{
				id:           id,
				name:         name,
				awsID:        aid,
				eurekaID:     cid,
				awsNamespace: ns,
				fromEureka:   sa.fromEureka || sb.fromEureka,
				fromAWS:      sa.fromAWS || sb.fromAWS,
			}
			if len(nodes) > 0 {
				s.nodes = nodes
			}
			if len(healths) > 0 {
				s.healths = healths
			}
			result[k] = s
		}
	}
	return result
}
