package catalog

import (
	"testing"

	_e "github.com/ArthurHlt/go-eureka-client/eureka"
	"github.com/stretchr/testify/require"
)

func TestEurekaRekeyHealths(t *testing.T) {
	type variant struct {
		nodes    map[string]map[int]node
		healths  map[string]health
		expected map[string]health
	}
	variants := []variant{
		{
			nodes:    map[string]map[int]node{},
			healths:  map[string]health{},
			expected: map[string]health{},
		},
		{
			nodes: map[string]map[int]node{
				"1.1.1.1": {8000: {port: 8000, awsID: "X1"}},
			},
			healths: map[string]health{
				"web_1.1.1.1_8000": up,
			},
			expected: map[string]health{
				"X1": up,
			},
		},
	}

	for _, v := range variants {
		e := eureka{
			services: map[string]service{
				"s1": {
					nodes:   v.nodes,
					healths: v.healths,
				},
			},
		}
		require.Equal(t, v.expected, e.rekeyHealths("s1", e.services["s1"].healths))
	}
}

func TestEurekaTransformServices(t *testing.T) {
	e := eureka{awsPrefix: "aws_"}

	services := _e.Applications{
		Applications: []_e.Application{
			{
				Name: "s1",
				Instances: []_e.InstanceInfo{
					{
						App:            "s1",
						HostName:       "s1-hostname",
						HomePageUrl:    "s1-homepageUrl",
						StatusPageUrl:  "s1-statuspageUrl",
						HealthCheckUrl: "s1-healthcheckUrl",
						IpAddr:         "127.0.0.1",
						InstanceID:     "i-nstanceIDs1",
						Status:         "UP",
						Port: &_e.Port{
							Port:    1,
							Enabled: true,
						},
						DataCenterInfo: &_e.DataCenterInfo{
							Metadata: &_e.DataCenterMetadata{
								InstanceId:       "i-nstanceIDs1",
								LocalIpv4:        "1.1.1.1",
								LocalHostname:    "s1-private-hostname",
								PublicHostname:   "s1-public-hostname",
								PublicIpv4:       "9.9.9.9",
								AvailabilityZone: "us-east-1e",
							},
						},
						LeaseInfo:  &_e.LeaseInfo{},
						Metadata:   &_e.MetaData{},
						SecurePort: &_e.Port{},
					},
				},
			},
			{
				Name: "s2",
				Instances: []_e.InstanceInfo{
					{
						App:            "s2",
						HostName:       "s2-hostname",
						HomePageUrl:    "s2-homepageUrl",
						StatusPageUrl:  "s2-statuspageUrl",
						HealthCheckUrl: "s2-healthcheckUrl",
						IpAddr:         "127.0.0.1",
						InstanceID:     "i-nstanceID",
						Status:         "OUT_OF_SERVICE",
						Port: &_e.Port{
							Port:    2,
							Enabled: true,
						},
						DataCenterInfo: &_e.DataCenterInfo{
							Metadata: &_e.DataCenterMetadata{
								InstanceId:       "i-nstanceIDs2",
								LocalIpv4:        "1.1.1.2",
								LocalHostname:    "s2-private-hostname",
								PublicHostname:   "s2-public-hostname",
								PublicIpv4:       "9.9.9.9",
								AvailabilityZone: "us-east-1e",
							},
						},
						LeaseInfo:  &_e.LeaseInfo{},
						Metadata:   &_e.MetaData{},
						SecurePort: &_e.Port{},
					},
				},
			},
		},
	}
	//services := map[string][]string{"s1": {"abc"}, "aws_s2": {EurekaAWSTag}}

	attributes_s1 := map[string]string{
		"local-ipv4":        "1.1.1.1",
		"local-hostname":    "s1-private-hostname",
		"public-hostname":   "s1-public-hostname",
		"public-ipv4":       "9.9.9.9",
		"availability-zone": "us-east-1e",
		"homePageUrl":       "s1-homepageUrl",
		"statusPageUrl":     "s1-statuspageUrl",
		"healthCheckUrl":    "s1-healthcheckUrl",
	}
	attributes_s2 := map[string]string{
		"local-ipv4":        "1.1.1.2",
		"local-hostname":    "s2-private-hostname",
		"public-hostname":   "s2-public-hostname",
		"public-ipv4":       "9.9.9.9",
		"availability-zone": "us-east-1e",
		"homePageUrl":       "s2-homepageUrl",
		"statusPageUrl":     "s2-statuspageUrl",
		"healthCheckUrl":    "s2-healthcheckUrl",
	}

	nodes_s1 := map[string]map[int]node{
		"1.1.1.1": {1: {port: 1, host: "1.1.1.1", awsID: "s1", eurekaID: "s1", instanceID: "i-nstanceIDs1", attributes: attributes_s1}},
	}

	nodes_s2 := map[string]map[int]node{
		// structure = ip : {port:{ ... }}
		"1.1.1.2": {2: {port: 2, host: "1.1.1.2", awsID: "s2", eurekaID: "s2", instanceID: "i-nstanceIDs2", attributes: attributes_s2}},
		//		"1.1.1.3": {3: {port: 3, host: "1.1.1.3", eurekaID: "s2", instanceID: "i-nstanceIDs2", attributes: attributes_s2}},
	}

	health_s1 := map[string]health{
		"i-nstanceIDs1": healthy,
	}
	health_s2 := map[string]health{
		"i-nstanceIDs2": unhealthy,
		//	"s3": unknown,
	}

	expected := map[string]service{
		"s1": {id: "s1", name: "s1", eurekaID: "s1", fromEureka: true, nodes: nodes_s1, healths: health_s1},
		"s2": {id: "s2", name: "s2", eurekaID: "s2", fromEureka: true, nodes: nodes_s2, healths: health_s2},
	}
	require.Equal(t, expected, e.transformServices(&services))
}
