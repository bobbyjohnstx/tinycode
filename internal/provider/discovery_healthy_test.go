package provider

import (
	"testing"

	"github.com/bobbyjohnstx/tinycode/internal/bus"
)

func TestShouldPoll_FilteredProviderSkipped(t *testing.T) {
	b := bus.New()
	defer b.Close()

	reg := NewRegistry()
	reg.SetFilters(nil, []string{"disabled-provider"})

	d := NewDiscovery(reg, b)

	if d.shouldPoll("disabled-provider") {
		t.Fatal("filtered provider should not be polled")
	}
	if !d.shouldPoll("enabled-provider") {
		t.Fatal("non-filtered provider should be polled")
	}
}
