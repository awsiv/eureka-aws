package catalog

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOnlyInFirst(t *testing.T) {
	type variant struct {
		a        map[string]service
		b        map[string]service
		expected map[string]service
	}

	table := []variant{
		{
			a:        map[string]service{},
			b:        map[string]service{},
			expected: map[string]service{},
		},
		{
			a: map[string]service{
				"s1": {fromEureka: true},
			},
			b: map[string]service{},
			expected: map[string]service{
				"s1": {fromEureka: true},
			},
		},
		{
			a: map[string]service{
				"s2": {fromEureka: true, nodes: map[string]map[int]node{"h1": {1: {}}}},
			},
			b: map[string]service{
				"s2": {nodes: map[string]map[int]node{"h2": {2: {}}}},
			},
			expected: map[string]service{
				"s2": {fromEureka: true, nodes: map[string]map[int]node{"h1": {1: {}}}},
			},
		},
		{
			a: map[string]service{
				"s3": {fromEureka: false, nodes: map[string]map[int]node{"h1": {1: {port: 1}}}},
			},
			b: map[string]service{
				"s3": {fromEureka: true, nodes: map[string]map[int]node{"h2": {2: {port: 2}}}},
			},
			expected: map[string]service{
				"s3": {fromEureka: true, nodes: map[string]map[int]node{"h1": {1: {port: 1}}}},
			},
		},
		{
			a: map[string]service{
				"s4": {fromAWS: true},
			},
			b: map[string]service{},
			expected: map[string]service{
				"s4": {fromAWS: true},
			},
		},
		{
			a: map[string]service{
				"s5": {fromAWS: true, nodes: map[string]map[int]node{"h1": {1: {port: 1}}}},
			},
			b: map[string]service{
				"s5": {nodes: map[string]map[int]node{"h2": {2: {port: 2}}}},
			},
			expected: map[string]service{
				"s5": {fromAWS: true, nodes: map[string]map[int]node{"h1": {1: {port: 1}}}},
			},
		},
		{
			a: map[string]service{
				"s6": {fromAWS: false, nodes: map[string]map[int]node{"h1": {1: {port: 1}}}},
			},
			b: map[string]service{
				"s6": {fromAWS: true, nodes: map[string]map[int]node{"h2": {2: {port: 2}}}},
			},
			expected: map[string]service{
				"s6": {fromAWS: true, nodes: map[string]map[int]node{"h1": {1: {port: 1}}}},
			},
		},
		{
			a:        map[string]service{"s7": {}},
			b:        map[string]service{},
			expected: map[string]service{"s7": {}},
		},
		{
			a:        map[string]service{"s8": {}},
			b:        map[string]service{"s8": {}},
			expected: map[string]service{},
		},
		{
			a:        map[string]service{"s9": {}, "s10": {}},
			b:        map[string]service{"s9": {}},
			expected: map[string]service{"s10": {}},
		},
		{
			a: map[string]service{
				"s11": {nodes: map[string]map[int]node{"h1": {1: {port: 1}}, "h2": {2: {port: 2}}}},
			},
			b: map[string]service{
				"s11": {nodes: map[string]map[int]node{"h1": {1: {port: 1}}, "h2": {2: {port: 2}}}},
			},
			expected: map[string]service{},
		},
		{
			a: map[string]service{
				"s12": {nodes: map[string]map[int]node{"h1": {1: {port: 1}}, "h2": {2: {port: 2}}}},
			},
			b: map[string]service{
				"s12": {nodes: map[string]map[int]node{"h2": {2: {port: 2}}}},
			},
			expected: map[string]service{
				"s12": {nodes: map[string]map[int]node{"h1": {1: {port: 1}}}},
			},
		},
		{
			a: map[string]service{
				"s13": {nodes: map[string]map[int]node{"h1": {1: {port: 1}}, "h2": {2: {port: 2}}}},
			},
			b: map[string]service{
				"s13": {awsID: "id", nodes: map[string]map[int]node{"h2": {2: {port: 2}}}},
			},
			expected: map[string]service{
				"s13": {awsID: "id", nodes: map[string]map[int]node{"h1": {1: {port: 1}}}},
			},
		},
		{
			a: map[string]service{
				"s14": {nodes: map[string]map[int]node{"h1": {1: {port: 1}}, "h2": {2: {port: 2}}}},
			},
			b: map[string]service{
				"s14": {awsNamespace: "ns1", nodes: map[string]map[int]node{"h2": {2: {port: 2}}}},
			},
			expected: map[string]service{
				"s14": {awsNamespace: "ns1", nodes: map[string]map[int]node{"h1": {1: {port: 1}}}},
			},
		},
		{
			a: map[string]service{
				"s15": {nodes: map[string]map[int]node{"h1": {1: {awsID: "a1"}}}},
			},
			b: map[string]service{
				"s15": {nodes: map[string]map[int]node{"h2": {}}},
			},
			expected: map[string]service{
				"s15": {nodes: map[string]map[int]node{"h1": {1: {awsID: "a1"}}}},
			},
		},
		{
			a: map[string]service{
				"s16": {healths: map[string]health{"h1": up, "h2": unhealthy}},
			},
			b: map[string]service{
				"s16": {healths: map[string]health{"h1": up}},
			},
			expected: map[string]service{
				"s16": {healths: map[string]health{"h2": unhealthy}},
			},
		},
		{
			a: map[string]service{
				"s17": {healths: map[string]health{"h1": up}},
			},
			b: map[string]service{
				"s17": {healths: map[string]health{"h1": unhealthy}},
			},
			expected: map[string]service{
				"s17": {healths: map[string]health{"h1": up}},
			},
		},
		{
			a: map[string]service{
				"s18": {healths: map[string]health{"h1": up, "h2": unhealthy}},
			},
			b: map[string]service{
				"s18": {healths: map[string]health{"h2": unhealthy, "h1": up}},
			},
			expected: map[string]service{},
		},
		{
			a: map[string]service{
				"s19": {nodes: map[string]map[int]node{"h1": {1: {port: 1}}, "h2": {2: {port: 2}}}},
			},
			b: map[string]service{
				"s19": {eurekaID: "id", nodes: map[string]map[int]node{"h2": {2: {port: 2}}}},
			},
			expected: map[string]service{
				"s19": {eurekaID: "id", nodes: map[string]map[int]node{"h1": {1: {port: 1}}}},
			},
		},
		{
			a: map[string]service{
				"s20": {nodes: map[string]map[int]node{"h1": {1: {port: 1}}, "h2": {2: {port: 2}}}},
			},
			b: map[string]service{
				"s20": {id: "id", name: "name", nodes: map[string]map[int]node{"h2": {2: {port: 2}}}},
			},
			expected: map[string]service{
				"s20": {id: "id", name: "name", nodes: map[string]map[int]node{"h1": {1: {port: 1}}}},
			},
		},
	}

	for _, v := range table {
		require.Equal(t, v.expected, onlyInFirst(v.a, v.b))
	}
}

func TestHostPortFromCheckID(t *testing.T) {
	host, port := hostPortFromID("service_abc_1.9.9.9_3333")
	require.Equal(t, "1.9.9.9", host)
	require.Equal(t, 3333, port)
}
