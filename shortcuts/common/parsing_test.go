package common_test

import (
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"
)

func TestParseTiers(t *testing.T) {
	layers, err := common.ParseTiers("100:10,200:25,500:50")
	if err != nil {
		t.Fatalf("ParseTiers: %v", err)
	}
	if len(layers) != 3 {
		t.Fatalf("len = %d, want 3", len(layers))
	}
	want := []common.Layer{
		{ConditionValue: 100, ObtainValue: 10},
		{ConditionValue: 200, ObtainValue: 25},
		{ConditionValue: 500, ObtainValue: 50},
	}
	for i, l := range layers {
		if l != want[i] {
			t.Errorf("layers[%d] = %v, want %v", i, l, want[i])
		}
	}
}

func TestParseTiers_Single(t *testing.T) {
	layers, err := common.ParseTiers("50:5")
	if err != nil {
		t.Fatalf("ParseTiers: %v", err)
	}
	if len(layers) != 1 || layers[0].ConditionValue != 50 || layers[0].ObtainValue != 5 {
		t.Errorf("unexpected result: %v", layers)
	}
}

func TestParseTiers_Invalid(t *testing.T) {
	cases := []string{"", "abc:10", "100:xyz", "100", "100:10:20"}
	for _, s := range cases {
		if _, err := common.ParseTiers(s); err == nil {
			t.Errorf("ParseTiers(%q) expected error, got nil", s)
		}
	}
}

func TestParseTiers_PercentOnObtain(t *testing.T) {
	layers, err := common.ParseTiers("3:50%")
	if err != nil {
		t.Fatalf("ParseTiers: %v", err)
	}
	if len(layers) != 1 || layers[0].ConditionValue != 3 || layers[0].ObtainValue != 50 {
		t.Errorf("unexpected result: %v", layers)
	}
}

func TestParseTiers_MixedPercent(t *testing.T) {
	layers, err := common.ParseTiers("3:50%,5:70,10:90%")
	if err != nil {
		t.Fatalf("ParseTiers: %v", err)
	}
	want := []common.Layer{
		{ConditionValue: 3, ObtainValue: 50},
		{ConditionValue: 5, ObtainValue: 70},
		{ConditionValue: 10, ObtainValue: 90},
	}
	if len(layers) != len(want) {
		t.Fatalf("len = %d, want %d", len(layers), len(want))
	}
	for i, l := range layers {
		if l != want[i] {
			t.Errorf("layers[%d] = %v, want %v", i, l, want[i])
		}
	}
}

func TestParseTiers_PercentWithWhitespace(t *testing.T) {
	layers, err := common.ParseTiers(" 3 : 50% , 5:70% ")
	if err != nil {
		t.Fatalf("ParseTiers: %v", err)
	}
	want := []common.Layer{
		{ConditionValue: 3, ObtainValue: 50},
		{ConditionValue: 5, ObtainValue: 70},
	}
	if len(layers) != len(want) {
		t.Fatalf("len = %d, want %d", len(layers), len(want))
	}
	for i, l := range layers {
		if l != want[i] {
			t.Errorf("layers[%d] = %v, want %v", i, l, want[i])
		}
	}
}

func TestLayersToMaps(t *testing.T) {
	layers := []common.Layer{{ConditionValue: 100, ObtainValue: 10}}
	maps := common.LayersToMaps(layers)
	if len(maps) != 1 {
		t.Fatalf("len = %d, want 1", len(maps))
	}
	m, ok := maps[0].(map[string]any)
	if !ok {
		t.Fatalf("type assertion failed")
	}
	if m["condition_value"] != "100" {
		t.Errorf("condition_value = %v (%T), want \"100\"", m["condition_value"], m["condition_value"])
	}
	if m["obtain_value"] != "10" {
		t.Errorf("obtain_value = %v (%T), want \"10\"", m["obtain_value"], m["obtain_value"])
	}
}

func TestLayersToMaps_Decimal(t *testing.T) {
	layers := []common.Layer{{ConditionValue: 100.5, ObtainValue: 10.25}}
	maps := common.LayersToMaps(layers)
	m := maps[0].(map[string]any)
	if m["condition_value"] != "100.5" {
		t.Errorf("condition_value = %v, want \"100.5\"", m["condition_value"])
	}
	if m["obtain_value"] != "10.25" {
		t.Errorf("obtain_value = %v, want \"10.25\"", m["obtain_value"])
	}
}

func TestParseProducts(t *testing.T) {
	got := common.ParseProducts("id1,id2,id3")
	want := []string{"id1", "id2", "id3"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestParseProducts_AllReturnsNil(t *testing.T) {
	if got := common.ParseProducts("all"); len(got) != 0 {
		t.Errorf("ParseProducts(all) = %v, want nil/empty", got)
	}
}

func TestParseProducts_EmptyReturnsNil(t *testing.T) {
	if got := common.ParseProducts(""); len(got) != 0 {
		t.Errorf("ParseProducts('') = %v, want nil/empty", got)
	}
}
