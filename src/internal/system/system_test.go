package system

import (
	"fmt"
	"sync"
	"testing"
)

func TestRuntimeValues(t *testing.T) {
	var info Info
	if info.Domain() != "" || info.TempFolder() != "" || info.Protocol() != "http" {
		t.Errorf("zero Info: domain %q, temp %q, protocol %q", info.Domain(), info.TempFolder(), info.Protocol())
	}

	info.SetDomain("192.168.1.2:34400")
	info.SetTempFolder("/tmp/xteve/")
	info.SetTLS(true)
	if info.Domain() != "192.168.1.2:34400" || info.TempFolder() != "/tmp/xteve/" || info.Protocol() != "https" {
		t.Errorf("Info: domain %q, temp %q, protocol %q", info.Domain(), info.TempFolder(), info.Protocol())
	}
}

// Run with -race.
func TestConcurrentUse(t *testing.T) {
	var info Info
	var wg sync.WaitGroup
	for g := range 8 {
		wg.Go(func() {
			for i := range 500 {
				info.SetDomain(fmt.Sprintf("host%d:%d", g, i))
				info.SetTLS(i%2 == 0)
				var end = info.BeginScan()
				_ = info.Domain() + info.Protocol() + info.TempFolder()
				end()
				end() // a second call has no effect
			}
		})
	}
	wg.Wait()

	if info.Scanning() {
		t.Error("Scanning after all scans ended")
	}
}
