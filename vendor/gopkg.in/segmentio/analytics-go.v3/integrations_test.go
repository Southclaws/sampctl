package analytics

import (
	"reflect"
	"testing"
)

func TestIntegrationsEnableAll(t *testing.T) {
	i0 := Integrations{"all": true}
	i1 := NewIntegrations().EnableAll()

	if !reflect.DeepEqual(i0, i1) {
		t.Errorf("calling EnableAll produced an invalid integration:\n- expected: %#v\n- found: %#v", i0, i1)
	}
}

func TestIntegrationsDisableAll(t *testing.T) {
	i0 := Integrations{"all": false}
	i1 := NewIntegrations().DisableAll()

	if !reflect.DeepEqual(i0, i1) {
		t.Errorf("calling DisableAll produced an invalid integration:\n- expected: %#v\n- found: %#v", i0, i1)
	}
}
