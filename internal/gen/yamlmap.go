package gen

import (
	"gopkg.in/yaml.v3"
)

// M is a YAML mapping that keeps key order, so generated configs read like hand-written ones.
type M []KV

type KV struct {
	K string
	V any
}

func (m M) MarshalYAML() (any, error) {
	n := &yaml.Node{Kind: yaml.MappingNode}
	for _, kv := range m {
		var v yaml.Node
		if err := v.Encode(kv.V); err != nil {
			return nil, err
		}
		n.Content = append(n.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: kv.K}, &v)
	}
	return n, nil
}

// Get returns the value for a key (used by tests and the apply step).
func (m M) Get(k string) any {
	for _, kv := range m {
		if kv.K == k {
			return kv.V
		}
	}
	return nil
}
