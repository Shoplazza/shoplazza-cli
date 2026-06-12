package discounts

import (
	"testing"

	"shoplazza-cli-v2/shortcuts/common"
)

func TestValidateLayerObtainValues(t *testing.T) {
	cases := []struct {
		name      string
		layers    []common.Layer
		isPercent bool
		wantErr   bool
	}{
		{"percent ok", []common.Layer{{ConditionValue: 2, ObtainValue: 30}, {ConditionValue: 3, ObtainValue: 50}}, true, false},
		{"percent boundary 1", []common.Layer{{ConditionValue: 2, ObtainValue: 1}}, true, false},
		{"percent boundary 99", []common.Layer{{ConditionValue: 2, ObtainValue: 99}}, true, false},
		{"percent over", []common.Layer{{ConditionValue: 2, ObtainValue: 30}, {ConditionValue: 3, ObtainValue: 120}}, true, true},
		{"percent zero", []common.Layer{{ConditionValue: 2, ObtainValue: 0}}, true, true},
		{"percent negative", []common.Layer{{ConditionValue: 2, ObtainValue: -5}}, true, true},
		{"amount ok", []common.Layer{{ConditionValue: 100, ObtainValue: 10}, {ConditionValue: 200, ObtainValue: 25}}, false, false},
		{"amount zero", []common.Layer{{ConditionValue: 100, ObtainValue: 0}}, false, true},
		{"amount negative", []common.Layer{{ConditionValue: 100, ObtainValue: -5}}, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateLayerObtainValues(tc.layers, tc.isPercent)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, tc.wantErr)
			}
		})
	}
}
